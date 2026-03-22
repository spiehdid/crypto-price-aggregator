package bitfinex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

var tickerSymbols = map[string]string{
	"bitcoin":  "tBTCUSD",
	"ethereum": "tETHUSD",
	"solana":   "tSOLUSD",
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
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		healthy: true,
	}
}

func (p *Provider) Name() string {
	return "bitfinex"
}

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return &model.ProviderStatus{
		Name:          "bitfinex",
		Healthy:       p.healthy,
		Tier:          model.TierFree,
		RemainingRate: p.cfg.RateLimit,
		LastError:     p.lastError,
		LastSuccessAt: p.lastSuccessAt,
	}
}

func (p *Provider) GetPrice(ctx context.Context, coinID, currency string) (*model.Price, error) {
	symbol, ok := tickerSymbols[coinID]
	if !ok {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}

	url := fmt.Sprintf("%s/v2/ticker/%s", p.cfg.BaseURL, symbol)

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

	// Response is an array: [bid, bid_size, ask, ask_size, daily_change, daily_change_pct, last_price, volume, high, low]
	var raw []float64
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Need at least 8 elements (index 7 = volume)
	if len(raw) < 8 {
		err := fmt.Errorf("unexpected response length: %d", len(raw))
		p.recordError(err)
		return nil, err
	}

	// Index 6 = last price, index 7 = volume, index 5 = daily_change_pct
	lastPrice := raw[6]
	volume := raw[7]
	changePct := raw[5]

	value := decimal.NewFromFloat(lastPrice)

	now := time.Now()
	price := &model.Price{
		CoinID:     coinID,
		Currency:   currency,
		Value:      value,
		Provider:   "bitfinex",
		Timestamp:  now,
		ReceivedAt: now,
		Volume24h:  decimal.NewFromFloat(volume),
		Change24h:  changePct * 100, // convert from decimal fraction to percentage
	}

	p.recordSuccess()
	return price, nil
}

func (p *Provider) GetPrices(ctx context.Context, coinIDs []string, currency string) ([]model.Price, error) {
	var prices []model.Price
	for _, id := range coinIDs {
		price, err := p.GetPrice(ctx, id, currency)
		if err != nil {
			return nil, err
		}
		prices = append(prices, *price)
	}
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
