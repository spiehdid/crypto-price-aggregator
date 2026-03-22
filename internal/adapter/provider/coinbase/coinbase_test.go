package coinbase_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coinbase"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/common"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/products/BTC-USD/ticker", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"price":"67432.15","volume":"3927.87","trade_id":984945216,"time":"2026-03-21T15:34:58Z"}`))
	}))
	defer server.Close()

	provider := coinbase.New(coinbase.Config{BaseURL: server.URL, RateLimit: 20})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "coinbase", price.Provider)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := coinbase.New(coinbase.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestName(t *testing.T) {
	provider := coinbase.New(coinbase.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "coinbase", provider.Name())
}

func TestGetPrice_CoinNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := coinbase.New(coinbase.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "unknowncoin", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := coinbase.New(coinbase.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}

func TestCoinbase_MalformedJSON(t *testing.T) {
	common.TestMalformedJSON(t, "coinbase", func(baseURL string) port.PriceProvider {
		return coinbase.New(coinbase.Config{BaseURL: baseURL})
	})
}

func TestCoinbase_EmptyBody(t *testing.T) {
	common.TestEmptyBody(t, "coinbase", func(baseURL string) port.PriceProvider {
		return coinbase.New(coinbase.Config{BaseURL: baseURL})
	})
}

func TestCoinbase_ServerErrors(t *testing.T) {
	common.TestServerErrors(t, "coinbase", func(baseURL string) port.PriceProvider {
		return coinbase.New(coinbase.Config{BaseURL: baseURL})
	})
}

func TestCoinbase_Timeout(t *testing.T) {
	common.TestTimeout(t, "coinbase", func(baseURL string) port.PriceProvider {
		return coinbase.New(coinbase.Config{BaseURL: baseURL})
	})
}
