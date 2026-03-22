package cryptocompare_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/cryptocompare"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/data/price", r.URL.Path)
		assert.Equal(t, "BTC", r.URL.Query().Get("fsym"))
		assert.Equal(t, "USD", r.URL.Query().Get("tsyms"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"USD":67432.15}`))
	}))
	defer server.Close()

	provider := cryptocompare.New(cryptocompare.Config{BaseURL: server.URL})
	price, err := provider.GetPrice(context.Background(), "BTC", "USD")
	require.NoError(t, err)
	assert.Equal(t, "BTC", price.CoinID)
	assert.Equal(t, "USD", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "cryptocompare", price.Provider)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	provider := cryptocompare.New(cryptocompare.Config{BaseURL: server.URL})
	_, err := provider.GetPrice(context.Background(), "BTC", "USD")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestGetPrice_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	provider := cryptocompare.New(cryptocompare.Config{BaseURL: server.URL})
	_, err := provider.GetPrice(context.Background(), "BTC", "USD")
	assert.ErrorIs(t, err, model.ErrProviderDown)
}

func TestName(t *testing.T) {
	provider := cryptocompare.New(cryptocompare.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "cryptocompare", provider.Name())
}

func TestStatus_Initial(t *testing.T) {
	provider := cryptocompare.New(cryptocompare.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.Equal(t, "cryptocompare", status.Name)
	assert.True(t, status.Healthy)
}

func TestGetPrices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"USD":67432.15}`))
	}))
	defer server.Close()

	provider := cryptocompare.New(cryptocompare.Config{BaseURL: server.URL})
	prices, err := provider.GetPrices(context.Background(), []string{"BTC", "ETH"}, "USD")
	require.NoError(t, err)
	assert.Len(t, prices, 2)
}
