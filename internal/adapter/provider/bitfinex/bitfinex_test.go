package bitfinex_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/bitfinex"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/ticker/tBTCUSD", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		// [bid, bid_size, ask, ask_size, daily_change, daily_change_pct, last_price, volume, high, low]
		_, _ = w.Write([]byte(`[67400.00, 1.5, 67432.15, 2.0, 150.0, 0.025, 67432.15, 1234.56, 68000.0, 66000.0]`))
	}))
	defer server.Close()

	provider := bitfinex.New(bitfinex.Config{BaseURL: server.URL})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "bitfinex", price.Provider)
	assert.False(t, price.Volume24h.IsZero())
	assert.NotZero(t, price.Change24h)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := bitfinex.New(bitfinex.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestGetPrice_UnknownCoin(t *testing.T) {
	provider := bitfinex.New(bitfinex.Config{BaseURL: "http://localhost"})

	_, err := provider.GetPrice(context.Background(), "nonexistent", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestGetPrice_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := bitfinex.New(bitfinex.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrProviderDown)
}

func TestName(t *testing.T) {
	provider := bitfinex.New(bitfinex.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "bitfinex", provider.Name())
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := bitfinex.New(bitfinex.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}

func TestGetPrices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[67400.00, 1.5, 67432.15, 2.0, 150.0, 0.025, 67432.15, 1234.56, 68000.0, 66000.0]`))
	}))
	defer server.Close()

	provider := bitfinex.New(bitfinex.Config{BaseURL: server.URL})
	prices, err := provider.GetPrices(context.Background(), []string{"bitcoin", "ethereum"}, "usd")
	require.NoError(t, err)
	assert.Len(t, prices, 2)
}
