package bingx

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

var coinSymbols = map[string]string{
	"bitcoin":  "BTC",
	"ethereum": "ETH",
	"solana":   "SOL",
	"cardano":  "ADA",
	"ripple":   "XRP",
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

func (p *Provider) Name() string { return "bingx" }

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return &model.ProviderStatus{
		Name:          "bingx",
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

type bingxTicker struct {
	Symbol             string  `json:"symbol"`
	LastPrice          float64 `json:"lastPrice"`
	HighPrice          float64 `json:"highPrice"`
	LowPrice           float64 `json:"lowPrice"`
	Volume             float64 `json:"volume"`
	PriceChangePercent string  `json:"priceChangePercent"`
}

type bingxResponse struct {
	Code int           `json:"code"`
	Data []bingxTicker `json:"data"`
}

func (p *Provider) GetPrices(ctx context.Context, coinIDs []string, currency string) ([]model.Price, error) {
	quote := strings.ToUpper(currency)
	if quote == "USD" {
		quote = "USDT"
	}

	now := time.Now()
	var prices []model.Price

	for _, coinID := range coinIDs {
		sym, ok := coinSymbols[strings.ToLower(coinID)]
		if !ok {
			continue
		}
		pair := sym + "-" + quote
		url := fmt.Sprintf("%s/openApi/spot/v1/ticker/24hr?symbol=%s&timestamp=%d", p.cfg.BaseURL, pair, time.Now().UnixMilli())

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

		var raw bingxResponse
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			p.recordError(err)
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		if len(raw.Data) == 0 {
			continue
		}

		ticker := raw.Data[0]
		val := decimal.NewFromFloat(ticker.LastPrice)
		if val.IsZero() {
			continue
		}

		pr := model.Price{
			CoinID:     coinID,
			Currency:   currency,
			Value:      val,
			Provider:   "bingx",
			Timestamp:  now,
			ReceivedAt: now,
		}
		pr.Volume24h = decimal.NewFromFloat(ticker.Volume)

		pctStr := strings.TrimSuffix(ticker.PriceChangePercent, "%")
		if pct, err := decimal.NewFromString(pctStr); err == nil {
			f, _ := pct.Float64()
			pr.Change24h = f
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
