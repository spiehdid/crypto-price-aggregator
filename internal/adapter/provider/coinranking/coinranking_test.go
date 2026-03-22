package coinranking_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coinranking"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/coin/Qwsogvtv82FCd", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("x-access-token"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"coin":{"symbol":"BTC","name":"Bitcoin","price":"67432.15","marketCap":"1320000000000","24hVolume":"25000000000","change":"-1.25"}}}`))
	}))
	defer server.Close()

	provider := coinranking.New(coinranking.Config{
		BaseURL: server.URL,
		APIKey:  "test-api-key",
	})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "coinranking", price.Provider)
	assert.False(t, price.MarketCap.IsZero())
	assert.False(t, price.Volume24h.IsZero())
	assert.NotZero(t, price.Change24h)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := coinranking.New(coinranking.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestGetPrice_UnknownCoin(t *testing.T) {
	provider := coinranking.New(coinranking.Config{BaseURL: "http://localhost"})

	_, err := provider.GetPrice(context.Background(), "nonexistent", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestGetPrice_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := coinranking.New(coinranking.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrUnauthorized)
}

func TestName(t *testing.T) {
	provider := coinranking.New(coinranking.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "coinranking", provider.Name())
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := coinranking.New(coinranking.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}

func TestGetPrices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"coin":{"symbol":"BTC","name":"Bitcoin","price":"67432.15","marketCap":"1320000000000","24hVolume":"25000000000","change":"-1.25"}}}`))
	}))
	defer server.Close()

	provider := coinranking.New(coinranking.Config{BaseURL: server.URL})
	prices, err := provider.GetPrices(context.Background(), []string{"bitcoin", "ethereum"}, "usd")
	require.NoError(t, err)
	assert.Len(t, prices, 2)
}
