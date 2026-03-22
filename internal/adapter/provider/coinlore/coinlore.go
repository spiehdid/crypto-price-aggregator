package coinlore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

var symbolToID = map[string]string{
	"bitcoin":  "90",
	"ethereum": "80",
	"solana":   "48543",
	"cardano":  "257",
	"ripple":   "58",
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
	return "coinlore"
}

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return &model.ProviderStatus{
		Name:          "coinlore",
		Healthy:       p.healthy,
		Tier:          model.TierFree,
		RemainingRate: p.cfg.RateLimit,
		LastError:     p.lastError,
		LastSuccessAt: p.lastSuccessAt,
	}
}

type tickerItem struct {
	ID           string `json:"id"`
	Symbol       string `json:"symbol"`
	Name         string `json:"name"`
	PriceUsd     string `json:"price_usd"`
	MarketCapUsd string `json:"market_cap_usd"`
	Volume24     string `json:"volume24"`
	Change24h    string `json:"percent_change_24h"`
}

func (p *Provider) GetPrice(ctx context.Context, coinID, currency string) (*model.Price, error) {
	numericID, ok := symbolToID[coinID]
	if !ok {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}

	url := fmt.Sprintf("%s/api/ticker/?id=%s", p.cfg.BaseURL, numericID)

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

	var items []tickerItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(items) == 0 {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}

	item := items[0]
	value, err := decimal.NewFromString(item.PriceUsd)
	if err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("parsing price: %w", err)
	}

	now := time.Now()
	price := &model.Price{
		CoinID:     coinID,
		Currency:   currency,
		Value:      value,
		Provider:   "coinlore",
		Timestamp:  now,
		ReceivedAt: now,
	}

	if mc, err := decimal.NewFromString(item.MarketCapUsd); err == nil {
		price.MarketCap = mc
	}
	if vol, err := decimal.NewFromString(item.Volume24); err == nil {
		price.Volume24h = vol
	}
	if ch, err := strconv.ParseFloat(item.Change24h, 64); err == nil {
		price.Change24h = ch
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
