package coingecko

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

type Config struct {
	BaseURL   string
	APIKey    string
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
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		healthy: true,
	}
}

func (p *Provider) Name() string {
	return "coingecko"
}

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return &model.ProviderStatus{
		Name:          "coingecko",
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

func (p *Provider) GetPrices(ctx context.Context, coinIDs []string, currency string) ([]model.Price, error) {
	url := fmt.Sprintf("%s/simple/price?ids=%s&vs_currencies=%s&include_market_cap=true&include_24hr_vol=true&include_24hr_change=true",
		p.cfg.BaseURL,
		strings.Join(coinIDs, ","),
		currency,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if p.cfg.APIKey != "" {
		req.Header.Set("x-cg-demo-api-key", p.cfg.APIKey)
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

	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	var raw map[string]map[string]json.Number
	if err := dec.Decode(&raw); err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	now := time.Now()
	var prices []model.Price

	for _, id := range coinIDs {
		data, ok := raw[id]
		if !ok {
			continue
		}
		numVal, ok := data[currency]
		if !ok {
			continue
		}
		val, err := decimal.NewFromString(string(numVal))
		if err != nil {
			p.recordError(err)
			return nil, fmt.Errorf("parsing price value: %w", err)
		}
		pr := model.Price{
			CoinID:     id,
			Currency:   currency,
			Value:      val,
			Provider:   "coingecko",
			Timestamp:  now,
			ReceivedAt: now,
		}
		if mcNum, ok := data[currency+"_market_cap"]; ok {
			if mc, err := decimal.NewFromString(string(mcNum)); err == nil {
				pr.MarketCap = mc
			}
		}
		if volNum, ok := data[currency+"_24h_vol"]; ok {
			if vol, err := decimal.NewFromString(string(volNum)); err == nil {
				pr.Volume24h = vol
			}
		}
		if changeNum, ok := data[currency+"_24h_change"]; ok {
			if f, err := changeNum.Float64(); err == nil {
				pr.Change24h = f
			}
		}
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
