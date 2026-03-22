package mexc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/common"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/mexc"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/ticker/price", r.URL.Path)
		assert.Equal(t, "BTCUSDT", r.URL.Query().Get("symbol"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","price":"67432.15"}`))
	}))
	defer server.Close()

	provider := mexc.New(mexc.Config{BaseURL: server.URL, RateLimit: 20})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "mexc", price.Provider)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := mexc.New(mexc.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestName(t *testing.T) {
	provider := mexc.New(mexc.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "mexc", provider.Name())
}

func TestGetPrice_CoinNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := mexc.New(mexc.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "unknowncoin", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := mexc.New(mexc.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}

func TestMEXC_MalformedJSON(t *testing.T) {
	common.TestMalformedJSON(t, "mexc", func(baseURL string) port.PriceProvider {
		return mexc.New(mexc.Config{BaseURL: baseURL})
	})
}

func TestMEXC_EmptyBody(t *testing.T) {
	common.TestEmptyBody(t, "mexc", func(baseURL string) port.PriceProvider {
		return mexc.New(mexc.Config{BaseURL: baseURL})
	})
}

func TestMEXC_ServerErrors(t *testing.T) {
	common.TestServerErrors(t, "mexc", func(baseURL string) port.PriceProvider {
		return mexc.New(mexc.Config{BaseURL: baseURL})
	})
}
