package livecoinwatch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/livecoinwatch"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/coins/single", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rate":67432.15,"volume":25000000000,"cap":1320000000000,"delta":{"day":0.985}}`))
	}))
	defer server.Close()

	provider := livecoinwatch.New(livecoinwatch.Config{
		BaseURL:   server.URL,
		RateLimit: 30,
	})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "livecoinwatch", price.Provider)
	assert.False(t, price.MarketCap.IsZero())
	assert.False(t, price.Volume24h.IsZero())
	// delta.day of 0.985 = -1.5%
	assert.InDelta(t, -1.5, price.Change24h, 0.0001)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := livecoinwatch.New(livecoinwatch.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestGetPrice_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := livecoinwatch.New(livecoinwatch.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrProviderDown)
}

func TestGetPrice_UnknownCoin(t *testing.T) {
	provider := livecoinwatch.New(livecoinwatch.Config{BaseURL: "http://localhost"})

	_, err := provider.GetPrice(context.Background(), "unknowncoin", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestName(t *testing.T) {
	provider := livecoinwatch.New(livecoinwatch.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "livecoinwatch", provider.Name())
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := livecoinwatch.New(livecoinwatch.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}
