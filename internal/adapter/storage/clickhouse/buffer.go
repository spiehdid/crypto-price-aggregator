package clickhouse

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type FlushFunc func(prices []model.Price) error

type BatchBuffer struct {
	mu          sync.Mutex
	prices      []model.Price
	maxSize     int
	interval    time.Duration
	flushFn     FlushFunc
	ticker      *time.Ticker
	done        chan struct{}
	wg          sync.WaitGroup
	flushErrors atomic.Int64
	flushCount  atomic.Int64
}

func NewBatchBuffer(maxSize int, interval time.Duration, flushFn FlushFunc) *BatchBuffer {
	return &BatchBuffer{
		prices:   make([]model.Price, 0, maxSize),
		maxSize:  maxSize,
		interval: interval,
		flushFn:  flushFn,
		done:     make(chan struct{}),
	}
}

func (b *BatchBuffer) Start() {
	b.wg.Add(1)
	b.ticker = time.NewTicker(b.interval)
	go func() {
		defer b.wg.Done()
		for {
			select {
			case <-b.ticker.C:
				b.flush()
			case <-b.done:
				return
			}
		}
	}()
}

func (b *BatchBuffer) Stop() {
	b.ticker.Stop()
	close(b.done)
	b.wg.Wait() // wait for goroutine to exit first
	b.flush()   // now safe — no concurrent flush
}

func (b *BatchBuffer) Add(price model.Price) {
	b.mu.Lock()
	b.prices = append(b.prices, price)
	shouldFlush := len(b.prices) >= b.maxSize
	b.mu.Unlock()

	if shouldFlush {
		b.flush()
	}
}

func (b *BatchBuffer) flush() {
	b.mu.Lock()
	if len(b.prices) == 0 {
		b.mu.Unlock()
		return
	}
	batch := b.prices
	b.prices = make([]model.Price, 0, b.maxSize)
	b.mu.Unlock()

	if err := b.flushFn(batch); err != nil {
		slog.Warn("clickhouse flush failed, retrying once", "count", len(batch), "error", err)
		time.Sleep(500 * time.Millisecond)
		if err := b.flushFn(batch); err != nil {
			b.flushErrors.Add(1)
			slog.Error("clickhouse flush retry failed, data lost", "count", len(batch), "error", err)
			return
		}
	}
	b.flushCount.Add(1)
}

func (b *BatchBuffer) FlushStats() (flushed, errors int64) {
	return b.flushCount.Load(), b.flushErrors.Load()
}
