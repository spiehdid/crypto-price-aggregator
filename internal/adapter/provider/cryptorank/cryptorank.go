package cryptorank

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

var coinSlugs = map[string]string{
	"bitcoin":  "bitcoin",
	"ethereum": "ethereum",
	"solana":   "solana",
	"cardano":  "cardano",
	"ripple":   "ripple",
}

type Config struct {
	BaseURL   string
	RateLimit int
	APIKey    string // optional
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

func (p *Provider) Name() string { return "cryptorank" }

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return &model.ProviderStatus{
		Name:          "cryptorank",
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

type cryptorankPriceUSD struct {
	USD float64 `json:"USD"`
}

type cryptorankCoin struct {
	Slug             string             `json:"slug"`
	Symbol           string             `json:"symbol"`
	Price            cryptorankPriceUSD `json:"price"`
	MarketCap        float64            `json:"marketCap"`
	Volume24h        float64            `json:"volume24h"`
	PercentChange24h float64            `json:"percentChange24h"`
}

type cryptorankResponse struct {
	Data cryptorankCoin `json:"data"`
}

func (p *Provider) GetPrices(ctx context.Context, coinIDs []string, currency string) ([]model.Price, error) {
	now := time.Now()
	var prices []model.Price

	for _, coinID := range coinIDs {
		slug, ok := coinSlugs[strings.ToLower(coinID)]
		if !ok {
			continue
		}

		url := fmt.Sprintf("%s/v0/coins/%s", p.cfg.BaseURL, slug)
		if p.cfg.APIKey != "" {
			url += "?api-key=" + p.cfg.APIKey
		}

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

		var raw cryptorankResponse
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			p.recordError(err)
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		val := decimal.NewFromFloat(raw.Data.Price.USD)
		if val.IsZero() {
			continue
		}

		pr := model.Price{
			CoinID:     coinID,
			Currency:   currency,
			Value:      val,
			Provider:   "cryptorank",
			Timestamp:  now,
			ReceivedAt: now,
		}
		pr.MarketCap = decimal.NewFromFloat(raw.Data.MarketCap)
		pr.Volume24h = decimal.NewFromFloat(raw.Data.Volume24h)
		pr.Change24h = raw.Data.PercentChange24h

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
