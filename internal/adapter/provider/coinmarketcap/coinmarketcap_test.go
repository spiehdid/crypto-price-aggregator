package coinmarketcap_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coinmarketcap"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/cryptocurrency/quotes/latest", r.URL.Path)
		assert.Equal(t, "BTC", r.URL.Query().Get("symbol"))
		assert.Equal(t, "USD", r.URL.Query().Get("convert"))
		assert.Equal(t, "test-key", r.Header.Get("X-CMC_PRO_API_KEY"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"BTC":[{"quote":{"USD":{"price":67432.15}}}]}}`))
	}))
	defer server.Close()

	provider := coinmarketcap.New(coinmarketcap.Config{BaseURL: server.URL, APIKey: "test-key"})
	price, err := provider.GetPrice(context.Background(), "BTC", "USD")
	require.NoError(t, err)
	assert.Equal(t, "BTC", price.CoinID)
	assert.Equal(t, "USD", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "coinmarketcap", price.Provider)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	provider := coinmarketcap.New(coinmarketcap.Config{BaseURL: server.URL, APIKey: "key"})
	_, err := provider.GetPrice(context.Background(), "BTC", "USD")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestGetPrice_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	provider := coinmarketcap.New(coinmarketcap.Config{BaseURL: server.URL, APIKey: "bad"})
	_, err := provider.GetPrice(context.Background(), "BTC", "USD")
	assert.ErrorIs(t, err, model.ErrUnauthorized)
}

func TestName(t *testing.T) {
	provider := coinmarketcap.New(coinmarketcap.Config{BaseURL: "http://localhost", APIKey: "key"})
	assert.Equal(t, "coinmarketcap", provider.Name())
}

func TestStatus_Initial(t *testing.T) {
	provider := coinmarketcap.New(coinmarketcap.Config{BaseURL: "http://localhost", APIKey: "key"})
	status := provider.Status()
	assert.Equal(t, "coinmarketcap", status.Name)
	assert.True(t, status.Healthy)
}

func TestGetPrices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"BTC":[{"quote":{"USD":{"price":67432.15}}}],"ETH":[{"quote":{"USD":{"price":3500.00}}}]}}`))
	}))
	defer server.Close()

	provider := coinmarketcap.New(coinmarketcap.Config{BaseURL: server.URL, APIKey: "test-key"})
	prices, err := provider.GetPrices(context.Background(), []string{"BTC", "ETH"}, "USD")
	require.NoError(t, err)
	assert.Len(t, prices, 2)
}
