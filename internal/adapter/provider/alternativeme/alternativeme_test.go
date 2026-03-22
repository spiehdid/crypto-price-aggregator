package alternativeme_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/alternativeme"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/ticker/1/", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"Bitcoin","symbol":"BTC","quotes":{"USD":{"price":67432.15,"volume_24h":26164068523,"market_cap":1413176625319,"percentage_change_24h":1.12}}}]}`))
	}))
	defer server.Close()

	provider := alternativeme.New(alternativeme.Config{BaseURL: server.URL, RateLimit: 20})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "alternativeme", price.Provider)
	assert.False(t, price.Volume24h.IsZero())
	assert.False(t, price.MarketCap.IsZero())
	assert.InDelta(t, 1.12, price.Change24h, 0.001)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := alternativeme.New(alternativeme.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestName(t *testing.T) {
	provider := alternativeme.New(alternativeme.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "alternativeme", provider.Name())
}

func TestGetPrice_CoinNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	provider := alternativeme.New(alternativeme.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "unknowncoin", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := alternativeme.New(alternativeme.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}
