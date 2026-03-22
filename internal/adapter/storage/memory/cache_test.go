package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/storage/memory"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_SetAndGet(t *testing.T) {
	c := memory.NewCache()
	ctx := context.Background()
	price := &model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(67000),
		Provider:   "coingecko",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}

	err := c.Set(ctx, price, 1*time.Minute)
	require.NoError(t, err)

	got, err := c.Get(ctx, "bitcoin", "usd")
	require.NoError(t, err)
	assert.Equal(t, price.Value, got.Value)
}

func TestCache_Miss(t *testing.T) {
	c := memory.NewCache()
	ctx := context.Background()

	got, err := c.Get(ctx, "bitcoin", "usd")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestCache_Expiry(t *testing.T) {
	c := memory.NewCache()
	ctx := context.Background()
	price := &model.Price{
		CoinID:   "bitcoin",
		Currency: "usd",
		Value:    decimal.NewFromFloat(67000),
	}

	err := c.Set(ctx, price, 50*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(60 * time.Millisecond)

	got, err := c.Get(ctx, "bitcoin", "usd")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestCache_Delete(t *testing.T) {
	c := memory.NewCache()
	ctx := context.Background()
	price := &model.Price{
		CoinID:   "bitcoin",
		Currency: "usd",
		Value:    decimal.NewFromFloat(67000),
	}

	_ = c.Set(ctx, price, 1*time.Minute)
	err := c.Delete(ctx, "bitcoin", "usd")
	require.NoError(t, err)

	got, err := c.Get(ctx, "bitcoin", "usd")
	assert.NoError(t, err)
	assert.Nil(t, got)
}
