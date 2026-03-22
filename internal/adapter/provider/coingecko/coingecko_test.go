package coingecko_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coingecko"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/common"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/simple/price", r.URL.Path)
		assert.Equal(t, "bitcoin", r.URL.Query().Get("ids"))
		assert.Equal(t, "usd", r.URL.Query().Get("vs_currencies"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bitcoin":{"usd":67432.15,"usd_market_cap":1320000000000,"usd_24h_vol":25000000000,"usd_24h_change":2.5}}`))
	}))
	defer server.Close()

	provider := coingecko.New(coingecko.Config{
		BaseURL:   server.URL,
		RateLimit: 30,
	})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "coingecko", price.Provider)
	assert.False(t, price.MarketCap.IsZero())
	assert.False(t, price.Volume24h.IsZero())
	assert.NotZero(t, price.Change24h)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := coingecko.New(coingecko.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestGetPrice_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := coingecko.New(coingecko.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrProviderDown)
}

func TestGetPrice_CoinNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	provider := coingecko.New(coingecko.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "nonexistent", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestGetPrices_MultipleCoins(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bitcoin":{"usd":67000},"ethereum":{"usd":3500}}`))
	}))
	defer server.Close()

	provider := coingecko.New(coingecko.Config{BaseURL: server.URL})

	prices, err := provider.GetPrices(context.Background(), []string{"bitcoin", "ethereum"}, "usd")
	require.NoError(t, err)
	assert.Len(t, prices, 2)
}

func TestName(t *testing.T) {
	provider := coingecko.New(coingecko.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "coingecko", provider.Name())
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := coingecko.New(coingecko.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}

func TestCoinGecko_MalformedJSON(t *testing.T) {
	common.TestMalformedJSON(t, "coingecko", func(baseURL string) port.PriceProvider {
		return coingecko.New(coingecko.Config{BaseURL: baseURL})
	})
}

func TestCoinGecko_EmptyBody(t *testing.T) {
	common.TestEmptyBody(t, "coingecko", func(baseURL string) port.PriceProvider {
		return coingecko.New(coingecko.Config{BaseURL: baseURL})
	})
}

func TestCoinGecko_ServerErrors(t *testing.T) {
	common.TestServerErrors(t, "coingecko", func(baseURL string) port.PriceProvider {
		return coingecko.New(coingecko.Config{BaseURL: baseURL})
	})
}

func TestCoinGecko_Timeout(t *testing.T) {
	common.TestTimeout(t, "coingecko", func(baseURL string) port.PriceProvider {
		return coingecko.New(coingecko.Config{BaseURL: baseURL})
	})
}
