package cryptorank_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/cryptorank"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/coins/bitcoin", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"slug":"bitcoin","symbol":"BTC","price":{"USD":67432.15},"marketCap":1320000000000,"volume24h":25000000000,"percentChange24h":2.5}}`))
	}))
	defer server.Close()

	provider := cryptorank.New(cryptorank.Config{BaseURL: server.URL, RateLimit: 20})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "cryptorank", price.Provider)
	assert.False(t, price.MarketCap.IsZero())
	assert.False(t, price.Volume24h.IsZero())
	assert.InDelta(t, 2.5, price.Change24h, 0.001)
}

func TestGetPrice_WithAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "mykey", r.URL.Query().Get("api-key"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"slug":"bitcoin","symbol":"BTC","price":{"USD":67432.15},"marketCap":1320000000000,"volume24h":25000000000,"percentChange24h":2.5}}`))
	}))
	defer server.Close()

	provider := cryptorank.New(cryptorank.Config{BaseURL: server.URL, APIKey: "mykey"})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := cryptorank.New(cryptorank.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestName(t *testing.T) {
	provider := cryptorank.New(cryptorank.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "cryptorank", provider.Name())
}

func TestGetPrice_CoinNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"slug":"","symbol":"","price":{"USD":0},"marketCap":0,"volume24h":0,"percentChange24h":0}}`))
	}))
	defer server.Close()

	provider := cryptorank.New(cryptorank.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "unknowncoin", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := cryptorank.New(cryptorank.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}
