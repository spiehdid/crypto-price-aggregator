package coingecko_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coingecko"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenProvider_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/simple/token_price/ethereum")
		_, _ = w.Write([]byte(`{"0xdac17f958d2ee523a2206206994597c13d831ec7":{"usd":1.0001}}`))
	}))
	defer server.Close()

	tp := coingecko.NewTokenProvider(server.URL)
	price, err := tp.GetPriceByAddress(context.Background(), "ethereum", "0xdac17f958d2ee523a2206206994597c13d831ec7", "usd")
	require.NoError(t, err)
	assert.True(t, decimal.NewFromFloat(1.0001).Equal(price.Value))
	assert.Equal(t, "coingecko", price.Provider)
}

func TestTokenProvider_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	tp := coingecko.NewTokenProvider(server.URL)
	_, err := tp.GetPriceByAddress(context.Background(), "ethereum", "0x0000", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestTokenProvider_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	tp := coingecko.NewTokenProvider(server.URL)
	_, err := tp.GetPriceByAddress(context.Background(), "ethereum", "0xabc", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestTokenProvider_SupportsChain(t *testing.T) {
	tp := coingecko.NewTokenProvider("http://localhost")
	assert.True(t, tp.SupportsChain("ethereum"))
}
