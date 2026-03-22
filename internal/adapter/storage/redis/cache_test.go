//go:build integration

package redis_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	rediscache "github.com/spiehdid/crypto-price-aggregator/internal/adapter/storage/redis"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestRedisAddr(t *testing.T) string {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set, skipping integration test")
	}
	return addr
}

func TestRedisCache_SetAndGet(t *testing.T) {
	ctx := context.Background()
	cache, err := rediscache.NewCache(ctx, rediscache.Config{Addr: getTestRedisAddr(t)})
	require.NoError(t, err)
	defer cache.Close()

	price := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(67000.50), Provider: "coingecko",
		Timestamp: time.Now(), ReceivedAt: time.Now(),
	}

	require.NoError(t, cache.Set(ctx, price, 1*time.Minute))

	got, err := cache.Get(ctx, "bitcoin", "usd")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, price.Value.Equal(got.Value))
}

func TestRedisCache_Miss(t *testing.T) {
	ctx := context.Background()
	cache, err := rediscache.NewCache(ctx, rediscache.Config{Addr: getTestRedisAddr(t)})
	require.NoError(t, err)
	defer cache.Close()

	got, err := cache.Get(ctx, "nonexistent", "usd")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestRedisCache_Delete(t *testing.T) {
	ctx := context.Background()
	cache, err := rediscache.NewCache(ctx, rediscache.Config{Addr: getTestRedisAddr(t)})
	require.NoError(t, err)
	defer cache.Close()

	price := &model.Price{CoinID: "del-test", Currency: "usd", Value: decimal.NewFromFloat(67000)}
	_ = cache.Set(ctx, price, 1*time.Minute)

	require.NoError(t, cache.Delete(ctx, "del-test", "usd"))

	got, err := cache.Get(ctx, "del-test", "usd")
	assert.NoError(t, err)
	assert.Nil(t, got)
}
