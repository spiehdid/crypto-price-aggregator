package kraken_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/common"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/kraken"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/0/public/Ticker", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":[],"result":{"XXBTZUSD":{"c":["67432.15","0.001"],"v":["5678","12345"]}}}`))
	}))
	defer server.Close()

	provider := kraken.New(kraken.Config{BaseURL: server.URL})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "usd", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "kraken", price.Provider)
	assert.False(t, price.Volume24h.IsZero())
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := kraken.New(kraken.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestGetPrice_UnknownCoin(t *testing.T) {
	provider := kraken.New(kraken.Config{BaseURL: "http://localhost"})

	_, err := provider.GetPrice(context.Background(), "nonexistent", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestGetPrice_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := kraken.New(kraken.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrProviderDown)
}

func TestName(t *testing.T) {
	provider := kraken.New(kraken.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "kraken", provider.Name())
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := kraken.New(kraken.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}

func TestGetPrices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Serve both XBTUSD (bitcoin) and ETHUSD (ethereum) — result key is the full pair name
		pair := r.URL.Query().Get("pair")
		switch pair {
		case "XBTUSD":
			_, _ = w.Write([]byte(`{"error":[],"result":{"XXBTZUSD":{"c":["67432.15","0.001"],"v":["5678","12345"]}}}`))
		case "ETHUSD":
			_, _ = w.Write([]byte(`{"error":[],"result":{"XETHZUSD":{"c":["3500.00","0.01"],"v":["1000","5000"]}}}`))
		default:
			_, _ = w.Write([]byte(`{"error":[],"result":{"XXBTZUSD":{"c":["67432.15","0.001"],"v":["5678","12345"]}}}`))
		}
	}))
	defer server.Close()

	provider := kraken.New(kraken.Config{BaseURL: server.URL})
	prices, err := provider.GetPrices(context.Background(), []string{"bitcoin", "ethereum"}, "usd")
	require.NoError(t, err)
	assert.Len(t, prices, 2)
}

func TestKraken_MalformedJSON(t *testing.T) {
	common.TestMalformedJSON(t, "kraken", func(baseURL string) port.PriceProvider {
		return kraken.New(kraken.Config{BaseURL: baseURL})
	})
}

func TestKraken_EmptyBody(t *testing.T) {
	common.TestEmptyBody(t, "kraken", func(baseURL string) port.PriceProvider {
		return kraken.New(kraken.Config{BaseURL: baseURL})
	})
}

func TestKraken_ServerErrors(t *testing.T) {
	common.TestServerErrors(t, "kraken", func(baseURL string) port.PriceProvider {
		return kraken.New(kraken.Config{BaseURL: baseURL})
	})
}
