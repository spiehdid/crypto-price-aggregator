package dexscreener

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

var coinNames = map[string]string{
	"bitcoin":  "Bitcoin",
	"ethereum": "Ethereum",
	"solana":   "Solana",
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

func (p *Provider) Name() string { return "dexscreener" }

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return &model.ProviderStatus{
		Name:          "dexscreener",
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

type dexscreenerBaseToken struct {
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
}

type dexscreenerVolume struct {
	H24 float64 `json:"h24"`
}

type dexscreenerPriceChange struct {
	H24 float64 `json:"h24"`
}

type dexscreenerPair struct {
	BaseToken   dexscreenerBaseToken   `json:"baseToken"`
	PriceUsd    string                 `json:"priceUsd"`
	Volume      dexscreenerVolume      `json:"volume"`
	PriceChange dexscreenerPriceChange `json:"priceChange"`
}

type dexscreenerResponse struct {
	Pairs []dexscreenerPair `json:"pairs"`
}

func (p *Provider) GetPrices(ctx context.Context, coinIDs []string, currency string) ([]model.Price, error) {
	now := time.Now()
	var prices []model.Price

	for _, coinID := range coinIDs {
		name, ok := coinNames[strings.ToLower(coinID)]
		if !ok {
			continue
		}
		url := fmt.Sprintf("%s/latest/dex/search?q=%s", p.cfg.BaseURL, name)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		resp, err := p.client.Do(req)
		if err != nil {
			p.recordError(model.ErrProviderDown)
			return nil, model.ErrProviderDown
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			_ = resp.Body.Close()
			p.recordError(model.ErrRateLimited)
			return nil, model.ErrRateLimited
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			_ = resp.Body.Close()
			p.recordError(model.ErrUnauthorized)
			return nil, model.ErrUnauthorized
		case resp.StatusCode >= 500:
			_ = resp.Body.Close()
			p.recordError(model.ErrProviderDown)
			return nil, model.ErrProviderDown
		case resp.StatusCode != http.StatusOK:
			_ = resp.Body.Close()
			err := fmt.Errorf("unexpected status: %d", resp.StatusCode)
			p.recordError(err)
			return nil, err
		}

		var raw dexscreenerResponse
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			_ = resp.Body.Close()
			p.recordError(err)
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		_ = resp.Body.Close()

		if len(raw.Pairs) == 0 {
			continue
		}

		pair := raw.Pairs[0]
		val, err := decimal.NewFromString(pair.PriceUsd)
		if err != nil {
			continue
		}

		pr := model.Price{
			CoinID:     coinID,
			Currency:   currency,
			Value:      val,
			Provider:   "dexscreener",
			Timestamp:  now,
			ReceivedAt: now,
		}
		pr.Volume24h = decimal.NewFromFloat(pair.Volume.H24)
		pr.Change24h = pair.PriceChange.H24

		prices = append(prices, pr)
	}

	if len(prices) == 0 && len(coinIDs) > 0 {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}

	p.recordSuccess()
	return prices, nil
}

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
