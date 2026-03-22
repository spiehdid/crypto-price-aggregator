package coingecko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type TokenProvider struct {
	baseURL string
	client  *http.Client
}

func NewTokenProvider(baseURL string) *TokenProvider {
	return &TokenProvider{baseURL: baseURL, client: &http.Client{Timeout: 10 * time.Second}}
}

func (p *TokenProvider) SupportsChain(chain string) bool { return true }

func (p *TokenProvider) GetPriceByAddress(ctx context.Context, chain, address, currency string) (*model.Price, error) {
	url := fmt.Sprintf("%s/api/v3/simple/token_price/%s?contract_addresses=%s&vs_currencies=%s",
		p.baseURL, chain, strings.ToLower(address), strings.ToLower(currency))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, model.ErrProviderDown
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, model.ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status: %d", resp.StatusCode)
	}

	// Response: {"0xaddr": {"usd": 1.0001}}
	var raw map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding: %w", err)
	}

	addr := strings.ToLower(address)
	data, ok := raw[addr]
	if !ok {
		return nil, model.ErrCoinNotFound
	}
	val, ok := data[strings.ToLower(currency)]
	if !ok {
		return nil, model.ErrCoinNotFound
	}

	now := time.Now()
	return &model.Price{
		CoinID: addr, Currency: currency,
		Value: decimal.NewFromFloat(val), Provider: "coingecko",
		Timestamp: now, ReceivedAt: now,
	}, nil
}
