package blockchain_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/blockchain"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ticker", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"USD":{"last":67432.15,"buy":67430,"sell":67435,"symbol":"$"},"EUR":{"last":62100.50,"buy":62098,"sell":62103,"symbol":"€"}}`))
	}))
	defer server.Close()

	provider := blockchain.New(blockchain.Config{BaseURL: server.URL, RateLimit: 20})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "USD")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", price.CoinID)
	assert.Equal(t, "USD", price.Currency)
	assert.True(t, decimal.NewFromFloat(67432.15).Equal(price.Value))
	assert.Equal(t, "blockchain", price.Provider)
}

func TestGetPrice_EUR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"USD":{"last":67432.15,"buy":67430,"sell":67435,"symbol":"$"},"EUR":{"last":62100.50,"buy":62098,"sell":62103,"symbol":"€"}}`))
	}))
	defer server.Close()

	provider := blockchain.New(blockchain.Config{BaseURL: server.URL})

	price, err := provider.GetPrice(context.Background(), "bitcoin", "EUR")
	require.NoError(t, err)
	assert.True(t, decimal.NewFromFloat(62100.50).Equal(price.Value))
}

func TestGetPrice_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := blockchain.New(blockchain.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "bitcoin", "USD")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestName(t *testing.T) {
	provider := blockchain.New(blockchain.Config{BaseURL: "http://localhost"})
	assert.Equal(t, "blockchain", provider.Name())
}

func TestGetPrice_NonBitcoinCoin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not be called for non-bitcoin coins.
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := blockchain.New(blockchain.Config{BaseURL: server.URL})

	_, err := provider.GetPrice(context.Background(), "ethereum", "USD")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestStatus_InitiallyHealthy(t *testing.T) {
	provider := blockchain.New(blockchain.Config{BaseURL: "http://localhost"})
	status := provider.Status()
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
}
