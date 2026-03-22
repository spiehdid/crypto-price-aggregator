package coinpaprika

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

func (p *Provider) Name() string { return "coinpaprika" }

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return &model.ProviderStatus{
		Name: "coinpaprika", Healthy: p.healthy, Tier: model.TierFree,
		LastError: p.lastError, LastSuccessAt: p.lastSuccessAt,
	}
}

type tickerResponse struct {
	ID     string `json:"id"`
	Quotes map[string]struct {
		Price float64 `json:"price"`
	} `json:"quotes"`
}

func (p *Provider) GetPrice(ctx context.Context, coinID, currency string) (*model.Price, error) {
	url := fmt.Sprintf("%s/v1/tickers/%s?quotes=%s", p.cfg.BaseURL, coinID, strings.ToUpper(currency))

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
	case resp.StatusCode == http.StatusNotFound:
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		p.recordError(model.ErrRateLimited)
		return nil, model.ErrRateLimited
	case resp.StatusCode >= 500:
		p.recordError(model.ErrProviderDown)
		return nil, model.ErrProviderDown
	case resp.StatusCode != http.StatusOK:
		err := fmt.Errorf("unexpected status: %d", resp.StatusCode)
		p.recordError(err)
		return nil, err
	}

	var raw tickerResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	cur := strings.ToUpper(currency)
	quote, ok := raw.Quotes[cur]
	if !ok {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}

	now := time.Now()
	p.recordSuccess()
	return &model.Price{
		CoinID: coinID, Currency: currency, Value: decimal.NewFromFloat(quote.Price),
		Provider: "coinpaprika", Timestamp: now, ReceivedAt: now,
	}, nil
}

func (p *Provider) GetPrices(ctx context.Context, coinIDs []string, currency string) ([]model.Price, error) {
	var prices []model.Price
	for _, id := range coinIDs {
		price, err := p.GetPrice(ctx, id, currency)
		if err != nil {
			continue
		}
		prices = append(prices, *price)
	}
	if len(prices) == 0 && len(coinIDs) > 0 {
		return nil, model.ErrCoinNotFound
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
