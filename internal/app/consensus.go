package app

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
)

type ConsensusConfig struct {
	Enabled      bool
	MinProviders int     // minimum providers needed for consensus
	MaxDeviation float64 // max % deviation from median to include in consensus
	Timeout      time.Duration
}

type ConsensusEngine struct {
	cfg ConsensusConfig
}

func NewConsensusEngine(cfg ConsensusConfig) *ConsensusEngine {
	return &ConsensusEngine{cfg: cfg}
}

type ConsensusResult struct {
	MedianPrice decimal.Decimal
	Prices      []model.Price // all collected prices
	Outliers    []model.Price // prices that deviated too much
	Confidence  float64       // 0.0-1.0 agreement ratio
}

func (e *ConsensusEngine) Compute(ctx context.Context, router port.ProviderRouter, coinID, currency string, maxProviders int) (*ConsensusResult, error) {
	if !e.cfg.Enabled || maxProviders <= 1 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()

	var mu sync.Mutex
	var prices []model.Price
	var wg sync.WaitGroup

	selected := make(map[string]bool)
	for i := 0; i < maxProviders; i++ {
		provider, err := router.Select(ctx, coinID)
		if err != nil {
			break
		}
		if selected[provider.Name()] {
			continue
		}
		selected[provider.Name()] = true

		wg.Add(1)
		go func(p port.PriceProvider) {
			defer wg.Done()
			start := time.Now()
			price, err := p.GetPrice(ctx, coinID, currency)
			elapsed := time.Since(start)
			if err != nil {
				router.ReportFailure(p.Name(), err)
				return
			}
			router.ReportSuccess(p.Name(), elapsed)

			mu.Lock()
			prices = append(prices, *price)
			mu.Unlock()
		}(provider)
	}

	wg.Wait()

	if len(prices) < e.cfg.MinProviders {
		slog.Warn("consensus: insufficient providers", "got", len(prices), "need", e.cfg.MinProviders)
		if len(prices) == 0 {
			return nil, model.ErrNoHealthyProvider
		}
		return &ConsensusResult{MedianPrice: prices[0].Value, Prices: prices, Confidence: 0}, nil
	}

	return e.computeResult(prices), nil
}

func (e *ConsensusEngine) computeResult(prices []model.Price) *ConsensusResult {
	sort.Slice(prices, func(i, j int) bool {
		return prices[i].Value.LessThan(prices[j].Value)
	})

	median := e.median(prices)

	if median.IsZero() {
		return &ConsensusResult{
			MedianPrice: median,
			Prices:      prices,
			Confidence:  0,
		}
	}

	maxDev := decimal.NewFromFloat(e.cfg.MaxDeviation)
	var valid, outliers []model.Price
	for _, p := range prices {
		dev := p.Value.Sub(median).Abs().Div(median).Mul(decimal.NewFromInt(100))
		if dev.GreaterThan(maxDev) {
			outliers = append(outliers, p)
		} else {
			valid = append(valid, p)
		}
	}

	confidence := float64(len(valid)) / float64(len(prices))

	return &ConsensusResult{
		MedianPrice: median,
		Prices:      prices,
		Outliers:    outliers,
		Confidence:  confidence,
	}
}

func (e *ConsensusEngine) median(prices []model.Price) decimal.Decimal {
	n := len(prices)
	if n == 0 {
		return decimal.Zero
	}
	if n%2 == 1 {
		return prices[n/2].Value
	}
	return prices[n/2-1].Value.Add(prices[n/2].Value).Div(decimal.NewFromInt(2))
}

func (e *ConsensusEngine) ComputeResultPublic(prices []model.Price) *ConsensusResult {
	return e.computeResult(prices)
}
