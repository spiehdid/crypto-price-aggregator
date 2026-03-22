package coincodex_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coincodex"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/coincodex/get_coin/btc", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"symbol":"BTC","coin_name":"Bitcoin","end":67432.15,"volume":"25000000000","cap":"1320000000000","change":"-1.25"}`))
	}))
	defer server.Close()

	provider := coincodex.New(coincodex.Config{
		BaseURL:   server.URL,
		RateLimit: 30,
	})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "coincodex", price.Provider)
	assert.False(t, price.MarketCap.IsZero())
	assert.False(t, price.Volume24h.IsZero())
	assert.InDelta(t, -1.25, price.Change24h, 0.0001)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := coincodex.New(coincodex.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestGetPrice_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := coincodex.New(coincodex.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrProviderDown)
}

func TestGetPrice_UnknownCoin(t *testing.T) {
	provider := coincodex.New(coincodex.Config{BaseURL: "http://localhost"})

	_, err := provider.GetPrice(context.Background(), "unknowncoin", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestName(t *testing.T) {
	provider := coincodex.New(coincodex.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "coincodex", provider.Name())
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := coincodex.New(coincodex.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}
