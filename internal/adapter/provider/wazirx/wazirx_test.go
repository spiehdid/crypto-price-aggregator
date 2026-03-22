package wazirx_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/wazirx"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sapi/v1/ticker/24hr", r.URL.Path)
		assert.Equal(t, "btcusdt", r.URL.Query().Get("symbol"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"symbol":"btcusdt","lastPrice":"67432.15","highPrice":"68000","lowPrice":"66000","volume":"0.01019","bidPrice":"67430","askPrice":"67435"}`))
	}))
	defer server.Close()

	provider := wazirx.New(wazirx.Config{BaseURL: server.URL, RateLimit: 20})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "wazirx", price.Provider)
	assert.False(t, price.Volume24h.IsZero())
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := wazirx.New(wazirx.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestName(t *testing.T) {
	provider := wazirx.New(wazirx.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "wazirx", provider.Name())
}

func TestGetPrice_CoinNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"symbol":"btcusdt","lastPrice":"0","highPrice":"0","lowPrice":"0","volume":"0","bidPrice":"0","askPrice":"0"}`))
	}))
	defer server.Close()

	provider := wazirx.New(wazirx.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "unknowncoin", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := wazirx.New(wazirx.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}
