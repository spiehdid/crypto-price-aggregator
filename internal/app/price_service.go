package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
	"github.com/spiehdid/crypto-price-aggregator/internal/registry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type PriceListener interface {
	OnPriceUpdate(price *model.Price)
}

// Cache and Store are optional (nil-safe).
type PriceServiceDeps struct {
	Router         port.ProviderRouter
	Cache          port.Cache
	Store          port.PriceStore
	CacheTTL       time.Duration
	MaxRetries     int
	Listener       PriceListener
	Validator      *PriceValidator
	Outlier        *OutlierDetector
	Consensus      *ConsensusEngine
	ResponseMode   string                    // "single", "validated", "consensus"
	Registry       *registry.TokenRegistry   // may be nil
	TokenProviders []port.TokenPriceProvider // may be empty
}

type PriceService struct {
	router         port.ProviderRouter
	cache          port.Cache
	store          port.PriceStore
	cacheTTL       time.Duration
	maxRetries     int
	listener       PriceListener
	validator      *PriceValidator
	outlier        *OutlierDetector
	consensus      *ConsensusEngine
	responseMode   string
	registry       *registry.TokenRegistry
	tokenProviders []port.TokenPriceProvider

	requestCount  atomic.Int64
	cacheHitCount atomic.Int64
	startTime     time.Time

	meter           metric.Meter
	requestCounter  metric.Int64Counter
	cacheHitCounter metric.Int64Counter
	providerLatency metric.Float64Histogram
	providerErrors  metric.Int64Counter
}

func NewPriceService(deps PriceServiceDeps) *PriceService {
	retries := deps.MaxRetries
	if retries <= 0 {
		retries = 1
	}
	s := &PriceService{
		router:         deps.Router,
		cache:          deps.Cache,
		store:          deps.Store,
		cacheTTL:       deps.CacheTTL,
		maxRetries:     retries,
		listener:       deps.Listener,
		validator:      deps.Validator,
		outlier:        deps.Outlier,
		consensus:      deps.Consensus,
		responseMode:   deps.ResponseMode,
		registry:       deps.Registry,
		tokenProviders: deps.TokenProviders,
		startTime:      time.Now(),
	}
	s.meter = otel.Meter("cpa.price_service")
	s.requestCounter, _ = s.meter.Int64Counter("cpa.requests.total")
	s.cacheHitCounter, _ = s.meter.Int64Counter("cpa.cache.hits.total")
	s.providerLatency, _ = s.meter.Float64Histogram("cpa.provider.latency.seconds")
	s.providerErrors, _ = s.meter.Int64Counter("cpa.provider.errors.total")
	return s
}

func (s *PriceService) GetPrice(ctx context.Context, coinID, currency string) (*model.Price, error) {
	s.requestCount.Add(1)
	s.requestCounter.Add(ctx, 1)

	if s.cache != nil {
		cached, err := s.cache.Get(ctx, coinID, currency)
		if err == nil && cached != nil {
			s.cacheHitCount.Add(1)
			s.cacheHitCounter.Add(ctx, 1)
			return cached, nil
		}
		if err != nil {
			slog.Warn("cache get error", "error", err)
		}
	}

	if s.responseMode == "consensus" && s.consensus != nil {
		result, err := s.consensus.Compute(ctx, s.router, coinID, currency, 5)
		if err == nil && result != nil && result.Confidence > 0 {
			now := time.Now()
			price := &model.Price{
				CoinID:     coinID,
				Currency:   currency,
				Value:      result.MedianPrice,
				Provider:   "consensus",
				Timestamp:  now,
				ReceivedAt: now,
			}
			if s.cache != nil {
				_ = s.cache.Set(ctx, price, s.cacheTTL)
			}
			if s.store != nil {
				go func() {
					if err := s.store.Save(context.Background(), price); err != nil {
						slog.Warn("store save failed", "error", err)
					}
				}()
			}
			if s.listener != nil {
				p := *price
				go s.listener.OnPriceUpdate(&p)
			}
			return price, nil
		}
		// Consensus failed — fall through to single-provider mode
		slog.Warn("consensus failed, falling back to single provider", "error", err)
	}

	tried := make(map[string]bool)
	var lastErr error
	for attempt := 0; attempt < s.maxRetries; attempt++ {
		provider, err := s.router.Select(ctx, coinID)
		if err != nil {
			lastErr = err
			break
		}

		name := provider.Name()
		if tried[name] {
			// Router keeps returning the same provider — no more alternatives
			break
		}
		tried[name] = true

		start := time.Now()
		price, err := provider.GetPrice(ctx, coinID, currency)
		elapsed := time.Since(start)

		if err != nil {
			s.router.ReportFailure(name, err)
			s.providerErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("provider", name)))
			lastErr = err
			continue
		}

		s.providerLatency.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attribute.String("provider", name)))
		s.router.ReportSuccess(name, elapsed)

		if s.validator != nil {
			if err := s.validator.Validate(price); err != nil {
				slog.Warn("price validation failed", "coin", coinID, "provider", name, "error", err)
				s.router.ReportFailure(name, err)
				lastErr = err
				continue // try next provider
			}

			if s.cache != nil {
				ref, _ := s.cache.Get(ctx, coinID, currency)
				if err := s.validator.ValidateAgainstReference(price, ref); err != nil {
					slog.Warn("price deviation too high", "coin", coinID, "provider", name, "error", err)
					// Don't hard fail — just log. The outlier detector handles scoring.
				}
			}
		}

		if s.cache != nil {
			if cacheErr := s.cache.Set(ctx, price, s.cacheTTL); cacheErr != nil {
				slog.Warn("cache set failed", "coin", coinID, "error", cacheErr)
			}
		}

		if s.store != nil {
			if storeErr := s.store.Save(ctx, price); storeErr != nil {
				slog.Warn("store save failed", "coin", coinID, "error", storeErr)
			}
		}

		if s.listener != nil {
			p := *price // copy to avoid race
			go s.listener.OnPriceUpdate(&p)
		}

		return price, nil
	}

	// All providers failed — try persistent store as last resort
	if s.store != nil {
		latest, _ := s.store.GetLatest(ctx, coinID, currency)
		if latest != nil {
			slog.Warn("serving stale price from store", "coin", coinID, "age", time.Since(latest.ReceivedAt))
			return latest, nil
		}
	}
	return nil, lastErr
}

func (s *PriceService) GetPrices(ctx context.Context, coinIDs []string, currency string) ([]model.Price, map[string]error) {
	var results []model.Price
	errs := make(map[string]error)

	for _, id := range coinIDs {
		price, err := s.GetPrice(ctx, id, currency)
		if err != nil {
			errs[id] = err
			continue
		}
		results = append(results, *price)
	}

	return results, errs
}

func (s *PriceService) GetOHLC(ctx context.Context, coinID, currency string, from, to time.Time, interval time.Duration) ([]model.OHLC, error) {
	if s.store == nil {
		return nil, fmt.Errorf("price store not configured")
	}
	return s.store.GetOHLC(ctx, coinID, currency, from, to, interval)
}

func (s *PriceService) Stats() (totalRequests, cacheHits int64, uptime time.Duration) {
	return s.requestCount.Load(), s.cacheHitCount.Load(), time.Since(s.startTime)
}
