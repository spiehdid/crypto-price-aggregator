package coincap_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coincap"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/assets/bitcoin", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"bitcoin","priceUsd":"67432.15","marketCapUsd":"1320000000000","volumeUsd24Hr":"25000000000","changePercent24Hr":"2.5"}}`))
	}))
	defer server.Close()

	provider := coincap.New(coincap.Config{BaseURL: server.URL})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "coincap", price.Provider)
	assert.False(t, price.MarketCap.IsZero())
	assert.False(t, price.Volume24h.IsZero())
	assert.NotZero(t, price.Change24h)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := coincap.New(coincap.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestGetPrice_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := coincap.New(coincap.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrProviderDown)
}

func TestGetPrice_CoinNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := coincap.New(coincap.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "nonexistent", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestName(t *testing.T) {
	provider := coincap.New(coincap.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "coincap", provider.Name())
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := coincap.New(coincap.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}

func TestGetPrices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"bitcoin","priceUsd":"67432.15","marketCapUsd":"1320000000000","volumeUsd24Hr":"25000000000","changePercent24Hr":"2.5"}}`))
	}))
	defer server.Close()

	provider := coincap.New(coincap.Config{BaseURL: server.URL})
	prices, err := provider.GetPrices(context.Background(), []string{"bitcoin", "ethereum"}, "usd")
	require.NoError(t, err)
	assert.Len(t, prices, 2)
}
