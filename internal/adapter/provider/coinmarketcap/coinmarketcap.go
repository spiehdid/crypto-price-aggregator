package coinmarketcap

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
	cfg           Config
	client        *http.Client
	mu            sync.RWMutex
	healthy       bool
	lastError     error
	lastSuccessAt time.Time
}

func New(cfg Config) *Provider {
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}, healthy: true}
}

func (p *Provider) Name() string { return "coinmarketcap" }

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return &model.ProviderStatus{
		Name: "coinmarketcap", Healthy: p.healthy, Tier: model.TierFree,
		LastError: p.lastError, LastSuccessAt: p.lastSuccessAt,
	}
}

func (p *Provider) GetPrice(ctx context.Context, coinID, currency string) (*model.Price, error) {
	prices, err := p.GetPrices(ctx, []string{coinID}, currency)
	if err != nil {
		return nil, err
	}
	if len(prices) == 0 {
		return nil, model.ErrCoinNotFound
	}
	return &prices[0], nil
}

type cmcResponse struct {
	Data map[string][]struct {
		Quote map[string]struct {
			Price float64 `json:"price"`
		} `json:"quote"`
	} `json:"data"`
}

func (p *Provider) GetPrices(ctx context.Context, coinIDs []string, currency string) ([]model.Price, error) {
	url := fmt.Sprintf("%s/v1/cryptocurrency/quotes/latest?symbol=%s&convert=%s",
		p.cfg.BaseURL, strings.Join(coinIDs, ","), strings.ToUpper(currency))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-CMC_PRO_API_KEY", p.cfg.APIKey)

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

	var raw cmcResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	now := time.Now()
	cur := strings.ToUpper(currency)
	var prices []model.Price
	for _, id := range coinIDs {
		entries, ok := raw.Data[strings.ToUpper(id)]
		if !ok || len(entries) == 0 {
			continue
		}
		quote, ok := entries[0].Quote[cur]
		if !ok {
			continue
		}
		prices = append(prices, model.Price{
			CoinID: id, Currency: currency, Value: decimal.NewFromFloat(quote.Price),
			Provider: "coinmarketcap", Timestamp: now, ReceivedAt: now,
		})
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
