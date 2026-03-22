package dexscreener_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/dexscreener"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenProvider_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"pairs":[{"priceUsd":"1.0001","baseToken":{"symbol":"USDT","name":"Tether"}}]}`))
	}))
	defer server.Close()

	tp := dexscreener.NewTokenProvider(server.URL)
	price, err := tp.GetPriceByAddress(context.Background(), "ethereum", "0xabc", "usd")
	require.NoError(t, err)
	assert.True(t, decimal.NewFromFloat(1.0001).Equal(price.Value))
	assert.Equal(t, "dexscreener", price.Provider)
}

func TestTokenProvider_NoPairs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"pairs":[]}`))
	}))
	defer server.Close()

	tp := dexscreener.NewTokenProvider(server.URL)
	_, err := tp.GetPriceByAddress(context.Background(), "ethereum", "0x0000", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestTokenProvider_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	tp := dexscreener.NewTokenProvider(server.URL)
	_, err := tp.GetPriceByAddress(context.Background(), "ethereum", "0xabc", "usd")
	assert.ErrorIs(t, err, model.ErrRateLimited)
}
