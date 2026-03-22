package coinranking

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

var coinUUIDs = map[string]string{
	"bitcoin":  "Qwsogvtv82FCd",
	"ethereum": "razxDUgYGNAdQ",
	"solana":   "zNZHO_Sjf",
}

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
	return "coinranking"
}

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return &model.ProviderStatus{
		Name:          "coinranking",
		Healthy:       p.healthy,
		Tier:          model.TierFree,
		RemainingRate: p.cfg.RateLimit,
		LastError:     p.lastError,
		LastSuccessAt: p.lastSuccessAt,
	}
}

type coinResponse struct {
	Status string `json:"status"`
	Data   struct {
		Coin struct {
			Symbol    string `json:"symbol"`
			Name      string `json:"name"`
			Price     string `json:"price"`
			MarketCap string `json:"marketCap"`
			Volume24h string `json:"24hVolume"`
			Change    string `json:"change"`
		} `json:"coin"`
	} `json:"data"`
}

func (p *Provider) GetPrice(ctx context.Context, coinID, currency string) (*model.Price, error) {
	uuid, ok := coinUUIDs[coinID]
	if !ok {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}

	url := fmt.Sprintf("%s/v2/coin/%s", p.cfg.BaseURL, uuid)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if p.cfg.APIKey != "" {
		req.Header.Set("x-access-token", p.cfg.APIKey)
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
	case resp.StatusCode == http.StatusNotFound:
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	case resp.StatusCode >= 500:
		p.recordError(model.ErrProviderDown)
		return nil, model.ErrProviderDown
	case resp.StatusCode != http.StatusOK:
		err := fmt.Errorf("unexpected status: %d", resp.StatusCode)
		p.recordError(err)
		return nil, err
	}

	var raw coinResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if raw.Data.Coin.Price == "" {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}

	value, err := decimal.NewFromString(raw.Data.Coin.Price)
	if err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("parsing price: %w", err)
	}

	now := time.Now()
	price := &model.Price{
		CoinID:     coinID,
		Currency:   currency,
		Value:      value,
		Provider:   "coinranking",
		Timestamp:  now,
		ReceivedAt: now,
	}

	if mc, err := decimal.NewFromString(raw.Data.Coin.MarketCap); err == nil {
		price.MarketCap = mc
	}
	if vol, err := decimal.NewFromString(raw.Data.Coin.Volume24h); err == nil {
		price.Volume24h = vol
	}
	if ch, err := strconv.ParseFloat(raw.Data.Coin.Change, 64); err == nil {
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
