package freecryptoapi

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

var coinToSymbol = map[string]string{
	"bitcoin":  "BTC",
	"ethereum": "ETH",
	"solana":   "SOL",
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
	return "freecryptoapi"
}

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return &model.ProviderStatus{
		Name:          "freecryptoapi",
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

type fcaSymbolData struct {
	Price            float64 `json:"price"`
	Volume           float64 `json:"volume"`
	MarketCap        float64 `json:"marketCap"`
	PercentChange24h float64 `json:"percentChange24h"`
}

type fcaResponse struct {
	Status int                      `json:"status"`
	Data   map[string]fcaSymbolData `json:"data"`
}

func (p *Provider) GetPrices(ctx context.Context, coinIDs []string, currency string) ([]model.Price, error) {
	now := time.Now()
	var prices []model.Price

	for _, coinID := range coinIDs {
		sym, ok := coinToSymbol[coinID]
		if !ok {
			continue
		}

		url := fmt.Sprintf("%s/v1/getData?symbol=%s", p.cfg.BaseURL, strings.ToUpper(sym))

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		if p.cfg.APIKey != "" {
			req.Header.Set("x-api-key", p.cfg.APIKey)
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

		var raw fcaResponse
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			p.recordError(err)
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		symData, ok := raw.Data[sym]
		if !ok {
			continue
		}

		price := model.Price{
			CoinID:     coinID,
			Currency:   currency,
			Value:      decimal.NewFromFloat(symData.Price),
			Provider:   "freecryptoapi",
			Timestamp:  now,
			ReceivedAt: now,
			MarketCap:  decimal.NewFromFloat(symData.MarketCap),
			Volume24h:  decimal.NewFromFloat(symData.Volume),
			Change24h:  symData.PercentChange24h,
		}
		prices = append(prices, price)
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
