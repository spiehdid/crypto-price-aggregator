package clickhouse_test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	chstore "github.com/spiehdid/crypto-price-aggregator/internal/adapter/storage/clickhouse"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
)

func TestBuffer_FlushOnSize(t *testing.T) {
	var flushed atomic.Int32
	flushFn := func(prices []model.Price) error {
		flushed.Add(int32(len(prices)))
		return nil
	}

	buf := chstore.NewBatchBuffer(3, 10*time.Second, flushFn)
	buf.Start()
	defer buf.Stop()

	for i := 0; i < 3; i++ {
		buf.Add(model.Price{CoinID: "bitcoin", Value: decimal.NewFromFloat(67000)})
	}

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(3), flushed.Load())
}

func TestBuffer_FlushOnInterval(t *testing.T) {
	var flushed atomic.Int32
	flushFn := func(prices []model.Price) error {
		flushed.Add(int32(len(prices)))
		return nil
	}

	buf := chstore.NewBatchBuffer(1000, 50*time.Millisecond, flushFn)
	buf.Start()
	defer buf.Stop()

	buf.Add(model.Price{CoinID: "bitcoin", Value: decimal.NewFromFloat(67000)})

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), flushed.Load())
}

func TestBuffer_FlushOnStop(t *testing.T) {
	var flushed atomic.Int32
	flushFn := func(prices []model.Price) error {
		flushed.Add(int32(len(prices)))
		return nil
	}

	buf := chstore.NewBatchBuffer(1000, 10*time.Second, flushFn)
	buf.Start()

	buf.Add(model.Price{CoinID: "bitcoin", Value: decimal.NewFromFloat(67000)})
	buf.Add(model.Price{CoinID: "ethereum", Value: decimal.NewFromFloat(3500)})

	buf.Stop()
	assert.Equal(t, int32(2), flushed.Load())
}

func TestBuffer_FlushStats_ErrorThenSuccess(t *testing.T) {
	var callCount atomic.Int32
	flushFn := func(prices []model.Price) error {
		n := callCount.Add(1)
		// First two calls (initial attempt + retry) both fail so flushErrors increments.
		// After that every call succeeds.
		if n <= 2 {
			return fmt.Errorf("simulated flush error (call %d)", n)
		}
		return nil
	}

	// maxSize=2 so adding 2 prices triggers an immediate size-based flush.
	buf := chstore.NewBatchBuffer(2, 10*time.Second, flushFn)
	buf.Start()

	// First batch — triggers size flush, which will fail on attempt + retry.
	// The retry inside flush() sleeps 500 ms, so we use Stop() to wait it out.
	buf.Add(model.Price{CoinID: "a", Value: decimal.NewFromFloat(1)})
	buf.Add(model.Price{CoinID: "b", Value: decimal.NewFromFloat(2)})

	// Give the retry goroutine time to finish (retry sleeps 500 ms inside flush).
	time.Sleep(700 * time.Millisecond)

	// Second batch — flush should now succeed.
	buf.Add(model.Price{CoinID: "c", Value: decimal.NewFromFloat(3)})
	buf.Add(model.Price{CoinID: "d", Value: decimal.NewFromFloat(4)})

	buf.Stop()

	flushed, errors := buf.FlushStats()
	// At least one error was recorded from the failed first batch.
	assert.GreaterOrEqual(t, errors, int64(1))
	// At least one successful flush was recorded for the second batch.
	assert.GreaterOrEqual(t, flushed, int64(1))
	assert.Greater(t, flushed+errors, int64(0))
}

func TestBuffer_FlushStats_AllSucceed(t *testing.T) {
	flushFn := func(prices []model.Price) error {
		return nil
	}

	buf := chstore.NewBatchBuffer(2, 10*time.Second, flushFn)
	buf.Start()

	buf.Add(model.Price{CoinID: "a", Value: decimal.NewFromFloat(1)})
	buf.Add(model.Price{CoinID: "b", Value: decimal.NewFromFloat(2)})

	buf.Stop()

	flushed, errors := buf.FlushStats()
	assert.Equal(t, int64(0), errors)
	// Two items triggered a size-based flush + Stop flushes remainder (empty), so at least 1.
	assert.GreaterOrEqual(t, flushed, int64(1))
}
