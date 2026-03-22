package coinpaprika_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coinpaprika"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/tickers/btc-bitcoin", r.URL.Path)
		assert.Equal(t, "USD", r.URL.Query().Get("quotes"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"btc-bitcoin","quotes":{"USD":{"price":67432.15}}}`))
	}))
	defer server.Close()

	provider := coinpaprika.New(coinpaprika.Config{BaseURL: server.URL})
	price, err := provider.GetPrice(context.Background(), "btc-bitcoin", "USD")
	require.NoError(t, err)
	assert.Equal(t, "btc-bitcoin", price.CoinID)
	assert.Equal(t, "USD", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "coinpaprika", price.Provider)
}

func TestGetPrice_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	provider := coinpaprika.New(coinpaprika.Config{BaseURL: server.URL})
	_, err := provider.GetPrice(context.Background(), "nonexistent", "USD")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	provider := coinpaprika.New(coinpaprika.Config{BaseURL: server.URL})
	_, err := provider.GetPrice(context.Background(), "btc-bitcoin", "USD")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestName(t *testing.T) {
	provider := coinpaprika.New(coinpaprika.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "coinpaprika", provider.Name())
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := coinpaprika.New(coinpaprika.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.Equal(t, "coinpaprika", status.Name)
	assert.True(t, status.Healthy)
}

func TestGetPrices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"btc-bitcoin","quotes":{"USD":{"price":67432.15}}}`))
	}))
	defer server.Close()

	provider := coinpaprika.New(coinpaprika.Config{BaseURL: server.URL})
	prices, err := provider.GetPrices(context.Background(), []string{"btc-bitcoin", "eth-ethereum"}, "USD")
	require.NoError(t, err)
	assert.NotEmpty(t, prices)
}
