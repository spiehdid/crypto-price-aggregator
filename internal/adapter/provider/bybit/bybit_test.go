package bybit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/bybit"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v5/market/tickers", r.URL.Path)
		assert.Equal(t, "spot", r.URL.Query().Get("category"))
		assert.Equal(t, "BTCUSDT", r.URL.Query().Get("symbol"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"retCode":0,"result":{"list":[{"symbol":"BTCUSDT","lastPrice":"67432.15","volume24h":"5678","turnover24h":"245000000","prevPrice24h":"66500","price24hPcnt":"0.014"}]}}`))
	}))
	defer server.Close()

	provider := bybit.New(bybit.Config{BaseURL: server.URL, RateLimit: 20})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "bybit", price.Provider)
	assert.False(t, price.Volume24h.IsZero())
	assert.InDelta(t, 1.4, price.Change24h, 0.001)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := bybit.New(bybit.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestName(t *testing.T) {
	provider := bybit.New(bybit.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "bybit", provider.Name())
}

func TestGetPrice_CoinNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"retCode":0,"result":{"list":[]}}`))
	}))
	defer server.Close()

	provider := bybit.New(bybit.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "unknowncoin", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := bybit.New(bybit.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}
