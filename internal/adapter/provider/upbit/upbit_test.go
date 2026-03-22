package upbit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/upbit"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/ticker", r.URL.Path)
		assert.Equal(t, "KRW-BTC", r.URL.Query().Get("markets"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"market":"KRW-BTC","trade_price":106126000.0,"signed_change_rate":0.0077,"acc_trade_volume_24h":676.55,"high_price":107000000,"low_price":105000000,"timestamp":1774107300873}]`))
	}))
	defer server.Close()

	provider := upbit.New(upbit.Config{BaseURL: server.URL, RateLimit: 20})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "krw")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "krw", price.Currency)
	assert.True(t, decimal.NewFromFloat(106126000.0).Equal(price.Value))
	assert.Equal(t, "upbit", price.Provider)
	assert.False(t, price.Volume24h.IsZero())
	assert.InDelta(t, 0.77, price.Change24h, 0.001)
}

func TestGetPrice_USDTMarket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "USDT-BTC", r.URL.Query().Get("markets"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"market":"USDT-BTC","trade_price":67432.15,"signed_change_rate":0.011,"acc_trade_volume_24h":100.0,"high_price":68000,"low_price":66000,"timestamp":1774107300873}]`))
	}))
	defer server.Close()

	provider := upbit.New(upbit.Config{BaseURL: server.URL})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := upbit.New(upbit.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "krw")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestName(t *testing.T) {
	provider := upbit.New(upbit.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "upbit", provider.Name())
}

func TestGetPrice_CoinNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	provider := upbit.New(upbit.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "unknowncoin", "krw")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := upbit.New(upbit.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}
