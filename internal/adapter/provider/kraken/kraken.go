package kraken

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

var krakenPairs = map[string]string{
	"bitcoin":  "XXBTZUSD",
	"ethereum": "XETHZUSD",
	"solana":   "SOLUSD",
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
	return "kraken"
}

func (p *Provider) Status() *model.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return &model.ProviderStatus{
		Name:          "kraken",
		Healthy:       p.healthy,
		Tier:          model.TierFree,
		RemainingRate: p.cfg.RateLimit,
		LastError:     p.lastError,
		LastSuccessAt: p.lastSuccessAt,
	}
}

type tickerResponse struct {
	Error  []string                   `json:"error"`
	Result map[string]json.RawMessage `json:"result"`
}

type tickerData struct {
	C []string `json:"c"` // last trade price [price, lot volume]
	V []string `json:"v"` // volume [today, last 24h]
}

func (p *Provider) GetPrice(ctx context.Context, coinID, currency string) (*model.Price, error) {
	pairName, ok := krakenPairs[coinID]
	if !ok {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}

	// Build the query pair name (XBT for BTC, etc.)
	var queryPair string
	switch coinID {
	case "bitcoin":
		queryPair = "XBTUSD"
	case "ethereum":
		queryPair = "ETHUSD"
	default:
		// For solana and others, strip the trailing USD and use short form
		queryPair = pairName
	}

	url := fmt.Sprintf("%s/0/public/Ticker?pair=%s", p.cfg.BaseURL, queryPair)

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

	var raw tickerResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(raw.Error) > 0 {
		err := fmt.Errorf("kraken error: %s", raw.Error[0])
		p.recordError(err)
		return nil, err
	}

	// The result key is the full pair name (e.g., XXBTZUSD)
	rawTicker, ok := raw.Result[pairName]
	if !ok {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}

	var ticker tickerData
	if err := json.Unmarshal(rawTicker, &ticker); err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("decoding ticker: %w", err)
	}

	if len(ticker.C) == 0 {
		p.recordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	}

	value, err := decimal.NewFromString(ticker.C[0])
	if err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("parsing price: %w", err)
	}

	now := time.Now()
	price := &model.Price{
		CoinID:     coinID,
		Currency:   currency,
		Value:      value,
		Provider:   "kraken",
		Timestamp:  now,
		ReceivedAt: now,
	}

	if len(ticker.V) >= 2 {
		if vol, err := decimal.NewFromString(ticker.V[1]); err == nil {
			price.Volume24h = vol
		}
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
