package alternativeme

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

var coinIDs = map[string]string{
	"bitcoin":  "1",
	"ethereum": "1027",
	"solana":   "5426",
	"cardano":  "2010",
	"ripple":   "52",
}

type Config struct {
	BaseURL   string
	RateLimit int
}

type Provider struct {
	cfg    Config
	client *http.Client

	mu            sync.RWMutex
	healthy       bool
	lastError     error
	lastSuccessAt time.Time
}

func New(cfg Config) *Provider {
	return &Provider{
		cfg:     cfg,
		client:  &http.Client{Timeout: 10 * time.Second},
		healthy: true,
	}
}

func (p *Provider) Name() string { return "alternativeme" }

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return &model.ProviderStatus{
		Name:          "alternativeme",
		Healthy:       p.healthy,
		Tier:          model.TierFree,
		RemainingRate: p.cfg.RateLimit,
		LastError:     p.lastError,
		LastSuccessAt: p.lastSuccessAt,
	}
}

func (p *Provider) GetPrice(ctx context.Context, coinID, currency string) (*model.Price, error) {
	prices, err := p.GetPrices(ctx, []string{coinID}, currency)
	if err != nil {
		return nil, err
	}
	if len(prices) == 0 {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}
	return &prices[0], nil
}

type altmeQuoteUSD struct {
	Price            float64 `json:"price"`
	Volume24h        float64 `json:"volume_24h"`
	MarketCap        float64 `json:"market_cap"`
	PercentChange24h float64 `json:"percentage_change_24h"`
}

type altmeQuotes struct {
	USD altmeQuoteUSD `json:"USD"`
}

type altmeCoin struct {
	ID     int         `json:"id"`
	Name   string      `json:"name"`
	Symbol string      `json:"symbol"`
	Quotes altmeQuotes `json:"quotes"`
}

type altmeResponse struct {
	Data []altmeCoin `json:"data"`
}

func (p *Provider) GetPrices(ctx context.Context, coinIDs []string, currency string) ([]model.Price, error) {
	now := time.Now()
	var prices []model.Price

	for _, coinID := range coinIDs {
		numID, ok := altmeCoinIDs[strings.ToLower(coinID)]
		if !ok {
			continue
		}

		url := fmt.Sprintf("%s/v2/ticker/%s/", p.cfg.BaseURL, numID)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		resp, err := p.client.Do(req)
		if err != nil {
			p.recordError(model.ErrProviderDown)
			return nil, model.ErrProviderDown
		}
		defer func() { _ = resp.Body.Close() }()

		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			p.recordError(model.ErrRateLimited)
			return nil, model.ErrRateLimited
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			p.recordError(model.ErrUnauthorized)
			return nil, model.ErrUnauthorized
		case resp.StatusCode >= 500:
			p.recordError(model.ErrProviderDown)
			return nil, model.ErrProviderDown
		case resp.StatusCode != http.StatusOK:
			err := fmt.Errorf("unexpected status: %d", resp.StatusCode)
			p.recordError(err)
			return nil, err
		}

		var raw altmeResponse
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			p.recordError(err)
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		if len(raw.Data) == 0 {
			continue
		}

		coin := raw.Data[0]
		val := decimal.NewFromFloat(coin.Quotes.USD.Price)
		if val.IsZero() {
			continue
		}

		pr := model.Price{
			CoinID:     coinID,
			Currency:   currency,
			Value:      val,
			Provider:   "alternativeme",
			Timestamp:  now,
			ReceivedAt: now,
		}
		pr.Volume24h = decimal.NewFromFloat(coin.Quotes.USD.Volume24h)
		pr.MarketCap = decimal.NewFromFloat(coin.Quotes.USD.MarketCap)
		pr.Change24h = coin.Quotes.USD.PercentChange24h

		prices = append(prices, pr)
	}

	if len(prices) == 0 && len(coinIDs) > 0 {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}

	p.recordSuccess()
	return prices, nil
}

var altmeCoinIDs = coinIDs

func (p *Provider) recordSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthy = true
	p.lastError = nil
	p.lastSuccessAt = time.Now()
}

func (p *Provider) recordError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthy = false
	p.lastError = err
}
