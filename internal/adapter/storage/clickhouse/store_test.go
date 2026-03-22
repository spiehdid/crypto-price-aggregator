//go:build integration

package clickhouse_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	chstore "github.com/spiehdid/crypto-price-aggregator/internal/adapter/storage/clickhouse"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestCHAddr(t *testing.T) string {
	addr := os.Getenv("TEST_CLICKHOUSE_ADDR")
	if addr == "" {
		t.Skip("TEST_CLICKHOUSE_ADDR not set")
	}
	return addr
}

func TestClickHouseStore_SaveAndGetLatest(t *testing.T) {
	ctx := context.Background()
	store, err := chstore.NewStore(ctx, chstore.Config{
		Addr: getTestCHAddr(t), Database: "default", Username: "default",
		BatchSize: 1, BatchInterval: 100 * time.Millisecond,
	})
	require.NoError(t, err)
	defer store.Close()

	now := time.Now()
	price := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(67000.50), Provider: "test",
		Timestamp: now, ReceivedAt: now,
	}
	require.NoError(t, store.Save(ctx, price))
	time.Sleep(200 * time.Millisecond)

	got, err := store.GetLatest(ctx, "bitcoin", "usd")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "bitcoin", got.CoinID)
}
