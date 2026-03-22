package dexscreener_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/common"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/dexscreener"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/latest/dex/search", r.URL.Path)
		assert.Equal(t, "Bitcoin", r.URL.Query().Get("q"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pairs":[{"baseToken":{"symbol":"BTC","name":"Bitcoin"},"priceUsd":"67432.15","volume":{"h24":25000000000},"priceChange":{"h24":2.5},"liquidity":{"usd":500000000}}]}`))
	}))
	defer server.Close()

	provider := dexscreener.New(dexscreener.Config{BaseURL: server.URL, RateLimit: 20})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "dexscreener", price.Provider)
	assert.False(t, price.Volume24h.IsZero())
	assert.InDelta(t, 2.5, price.Change24h, 0.001)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := dexscreener.New(dexscreener.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestName(t *testing.T) {
	provider := dexscreener.New(dexscreener.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "dexscreener", provider.Name())
}

func TestGetPrice_CoinNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pairs":[]}`))
	}))
	defer server.Close()

	provider := dexscreener.New(dexscreener.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "unknowncoin", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := dexscreener.New(dexscreener.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}

func TestDexScreener_MalformedJSON(t *testing.T) {
	common.TestMalformedJSON(t, "dexscreener", func(baseURL string) port.PriceProvider {
		return dexscreener.New(dexscreener.Config{BaseURL: baseURL})
	})
}

func TestDexScreener_EmptyBody(t *testing.T) {
	common.TestEmptyBody(t, "dexscreener", func(baseURL string) port.PriceProvider {
		return dexscreener.New(dexscreener.Config{BaseURL: baseURL})
	})
}

func TestDexScreener_ServerErrors(t *testing.T) {
	common.TestServerErrors(t, "dexscreener", func(baseURL string) port.PriceProvider {
		return dexscreener.New(dexscreener.Config{BaseURL: baseURL})
	})
}
