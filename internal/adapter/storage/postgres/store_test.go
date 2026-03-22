//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/storage/postgres"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestDSN(t *testing.T) string {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set, skipping integration test")
	}
	return dsn
}

func TestPostgresStore_SaveAndGetLatest(t *testing.T) {
	ctx := context.Background()
	store, err := postgres.NewStore(ctx, getTestDSN(t))
	require.NoError(t, err)
	defer store.Close()

	now := time.Now().Truncate(time.Microsecond)
	price := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(67000.50), Provider: "coingecko",
		Timestamp: now, ReceivedAt: now,
	}

	require.NoError(t, store.Save(ctx, price))

	got, err := store.GetLatest(ctx, "bitcoin", "usd")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "bitcoin", got.CoinID)
	assert.True(t, price.Value.Equal(got.Value))
}

func TestPostgresStore_GetHistory(t *testing.T) {
	ctx := context.Background()
	store, err := postgres.NewStore(ctx, getTestDSN(t))
	require.NoError(t, err)
	defer store.Close()

	now := time.Now().Truncate(time.Microsecond)
	for i := 0; i < 5; i++ {
		p := &model.Price{
			CoinID: "history-test", Currency: "usd",
			Value: decimal.NewFromFloat(67000 + float64(i)*100), Provider: "test",
			Timestamp:  now.Add(time.Duration(i) * time.Minute),
			ReceivedAt: now.Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, store.Save(ctx, p))
	}

	history, err := store.GetHistory(ctx, "history-test", "usd",
		now.Add(1*time.Minute), now.Add(3*time.Minute))
	require.NoError(t, err)
	assert.Len(t, history, 2)
}
