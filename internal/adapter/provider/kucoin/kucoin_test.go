package kucoin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/kucoin"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/market/orderbook/level1", r.URL.Path)
		assert.Equal(t, "BTC-USDT", r.URL.Query().Get("symbol"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"200000","data":{"price":"67432.15","time":1700000000000}}`))
	}))
	defer server.Close()

	provider := kucoin.New(kucoin.Config{BaseURL: server.URL})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usdt")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usdt", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "kucoin", price.Provider)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := kucoin.New(kucoin.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usdt")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestGetPrice_UnknownCoin(t *testing.T) {
	provider := kucoin.New(kucoin.Config{BaseURL: "http://localhost"})

	_, err := provider.GetPrice(context.Background(), "nonexistent", "usdt")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestGetPrice_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := kucoin.New(kucoin.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usdt")
	assert.ErrorIs(t, err, model.ErrProviderDown)
}

func TestName(t *testing.T) {
	provider := kucoin.New(kucoin.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "kucoin", provider.Name())
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := kucoin.New(kucoin.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}

func TestGetPrices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"200000","data":{"price":"67432.15","time":1700000000000}}`))
	}))
	defer server.Close()

	provider := kucoin.New(kucoin.Config{BaseURL: server.URL})
	prices, err := provider.GetPrices(context.Background(), []string{"bitcoin", "ethereum"}, "usdt")
	require.NoError(t, err)
	assert.Len(t, prices, 2)
}
