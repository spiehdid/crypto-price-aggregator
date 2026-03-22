package cryptodotcom_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/cryptodotcom"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/exchange/v1/public/get-tickers", r.URL.Path)
		assert.Equal(t, "BTC_USD", r.URL.Query().Get("instrument_name"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"result":{"data":[{"i":"BTC_USD","a":"67432.15","h":"68000","l":"66000","v":"5678"}]}}`))
	}))
	defer server.Close()

	provider := cryptodotcom.New(cryptodotcom.Config{BaseURL: server.URL, RateLimit: 20})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "cryptodotcom", price.Provider)
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := cryptodotcom.New(cryptodotcom.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestName(t *testing.T) {
	provider := cryptodotcom.New(cryptodotcom.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "cryptodotcom", provider.Name())
}

func TestGetPrice_CoinNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := cryptodotcom.New(cryptodotcom.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "unknowncoin", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := cryptodotcom.New(cryptodotcom.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}
