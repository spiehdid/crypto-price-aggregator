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

func TestStore_SaveAndGetLatest(t *testing.T) {
	s := memory.NewStore()
	ctx := context.Background()

	old := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(66000), ReceivedAt: time.Now().Add(-1 * time.Minute),
	}
	latest := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(67000), ReceivedAt: time.Now(),
	}

	require.NoError(t, s.Save(ctx, old))
	require.NoError(t, s.Save(ctx, latest))

	got, err := s.GetLatest(ctx, "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, latest.Value.Equal(got.Value))
}

func TestStore_GetLatest_NotFound(t *testing.T) {
	s := memory.NewStore()
	ctx := context.Background()

	got, err := s.GetLatest(ctx, "bitcoin", "usd")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestStore_SaveBatch(t *testing.T) {
	s := memory.NewStore()
	ctx := context.Background()

	prices := []model.Price{
		{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(67000), ReceivedAt: time.Now()},
		{CoinID: "ethereum", Currency: "usd", Value: decimal.NewFromFloat(3500), ReceivedAt: time.Now()},
	}

	require.NoError(t, s.SaveBatch(ctx, prices))

	btc, err := s.GetLatest(ctx, "bitcoin", "usd")
	require.NoError(t, err)
	require.NotNil(t, btc)

	eth, err := s.GetLatest(ctx, "ethereum", "usd")
	require.NoError(t, err)
	require.NotNil(t, eth)
}

func TestStore_GetHistory(t *testing.T) {
	s := memory.NewStore()
	ctx := context.Background()
	now := time.Now()

	for i := 0; i < 5; i++ {
		p := &model.Price{
			CoinID: "bitcoin", Currency: "usd",
			Value:      decimal.NewFromFloat(67000 + float64(i)*100),
			ReceivedAt: now.Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, s.Save(ctx, p))
	}

	history, err := s.GetHistory(ctx, "bitcoin", "usd", now.Add(1*time.Minute), now.Add(3*time.Minute))
	require.NoError(t, err)
	assert.Len(t, history, 2)
}

func TestStore_GetOHLC(t *testing.T) {
	s := memory.NewStore()
	ctx := context.Background()
	now := time.Now().Truncate(time.Hour) // align to hour

	// Create prices across 2 hours
	prices := []struct {
		value float64
		at    time.Duration
	}{
		{67000, 0},
		{67500, 10 * time.Minute},
		{66800, 20 * time.Minute},
		{67200, 50 * time.Minute},
		{68000, 1*time.Hour + 10*time.Minute},
		{67800, 1*time.Hour + 30*time.Minute},
	}

	for _, p := range prices {
		price := &model.Price{
			CoinID: "bitcoin", Currency: "usd",
			Value:      decimal.NewFromFloat(p.value),
			ReceivedAt: now.Add(p.at),
		}
		require.NoError(t, s.Save(ctx, price))
	}

	ohlc, err := s.GetOHLC(ctx, "bitcoin", "usd", now, now.Add(2*time.Hour), time.Hour)
	require.NoError(t, err)
	require.Len(t, ohlc, 2)

	// First hour
	assert.True(t, decimal.NewFromFloat(67000).Equal(ohlc[0].Open))
	assert.True(t, decimal.NewFromFloat(67500).Equal(ohlc[0].High))
	assert.True(t, decimal.NewFromFloat(66800).Equal(ohlc[0].Low))
	assert.True(t, decimal.NewFromFloat(67200).Equal(ohlc[0].Close))

	// Second hour
	assert.True(t, decimal.NewFromFloat(68000).Equal(ohlc[1].Open))
	assert.True(t, decimal.NewFromFloat(68000).Equal(ohlc[1].High))
	assert.True(t, decimal.NewFromFloat(67800).Equal(ohlc[1].Close))
}
