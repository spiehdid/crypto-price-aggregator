package dexscreener

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

func (p *TokenProvider) SupportsChain(_ string) bool { return true }

type dexTokenResponse struct {
	Pairs []struct {
		PriceUsd  string `json:"priceUsd"`
		BaseToken struct {
			Symbol string `json:"symbol"`
			Name   string `json:"name"`
		} `json:"baseToken"`
	} `json:"pairs"`
}

func (p *TokenProvider) GetPriceByAddress(ctx context.Context, chain, address, currency string) (*model.Price, error) {
	url := fmt.Sprintf("%s/tokens/v1/%s/%s", p.baseURL, chain, strings.ToLower(address))

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

	var raw dexTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding: %w", err)
	}

	if len(raw.Pairs) == 0 || raw.Pairs[0].PriceUsd == "" {
		return nil, model.ErrCoinNotFound
	}

	price, err := decimal.NewFromString(raw.Pairs[0].PriceUsd)
	if err != nil {
		return nil, fmt.Errorf("parsing price: %w", err)
	}

	now := time.Now()
	return &model.Price{
		CoinID: strings.ToLower(address), Currency: "usd",
		Value: price, Provider: "dexscreener",
		Timestamp: now, ReceivedAt: now,
	}, nil
}
