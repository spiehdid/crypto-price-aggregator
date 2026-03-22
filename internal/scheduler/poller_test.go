package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPriceFetcher struct {
	callCount atomic.Int32
}

func (m *mockPriceFetcher) FetchPrice(ctx context.Context, coinID, currency string) (*model.Price, error) {
	m.callCount.Add(1)
	return &model.Price{
		CoinID: coinID, Currency: currency,
		Value: decimal.NewFromFloat(67000), Provider: "mock",
		Timestamp: time.Now(), ReceivedAt: time.Now(),
	}, nil
}

func TestPoller_Subscribe_PollsAtInterval(t *testing.T) {
	fetcher := &mockPriceFetcher{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := scheduler.NewPoller(ctx, fetcher, 10*time.Millisecond)
	err := p.Subscribe(scheduler.Subscription{CoinID: "bitcoin", Currency: "usd", Interval: 20 * time.Millisecond})
	require.NoError(t, err)
	time.Sleep(70 * time.Millisecond)
	p.Stop()
	calls := fetcher.callCount.Load()
	assert.GreaterOrEqual(t, calls, int32(2), "expected at least 2 poll calls")
}

func TestPoller_Unsubscribe_StopsPolling(t *testing.T) {
	fetcher := &mockPriceFetcher{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := scheduler.NewPoller(ctx, fetcher, 10*time.Millisecond)
	_ = p.Subscribe(scheduler.Subscription{CoinID: "bitcoin", Currency: "usd", Interval: 20 * time.Millisecond})
	time.Sleep(50 * time.Millisecond)
	callsBefore := fetcher.callCount.Load()
	_ = p.Unsubscribe("bitcoin", "usd")
	time.Sleep(50 * time.Millisecond)
	callsAfter := fetcher.callCount.Load()
	assert.LessOrEqual(t, callsAfter-callsBefore, int32(1), "should stop polling after unsubscribe")
}

func TestPoller_Subscriptions_ListsActive(t *testing.T) {
	fetcher := &mockPriceFetcher{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := scheduler.NewPoller(ctx, fetcher, 10*time.Millisecond)
	_ = p.Subscribe(scheduler.Subscription{CoinID: "bitcoin", Currency: "usd", Interval: 1 * time.Second})
	_ = p.Subscribe(scheduler.Subscription{CoinID: "ethereum", Currency: "usd", Interval: 1 * time.Second})
	subs := p.Subscriptions()
	assert.Len(t, subs, 2)
	p.Stop()
}

func TestPoller_Subscribe_RespectsMinInterval(t *testing.T) {
	fetcher := &mockPriceFetcher{}
	minInterval := 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := scheduler.NewPoller(ctx, fetcher, minInterval)
	_ = p.Subscribe(scheduler.Subscription{CoinID: "bitcoin", Currency: "usd", Interval: 5 * time.Millisecond})
	subs := p.Subscriptions()
	assert.Equal(t, minInterval, subs[0].Interval)
	p.Stop()
}

func TestPoller_DuplicateSubscribe_UpdatesInterval(t *testing.T) {
	fetcher := &mockPriceFetcher{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := scheduler.NewPoller(ctx, fetcher, 10*time.Millisecond)
	_ = p.Subscribe(scheduler.Subscription{CoinID: "bitcoin", Currency: "usd", Interval: 100 * time.Millisecond})
	_ = p.Subscribe(scheduler.Subscription{CoinID: "bitcoin", Currency: "usd", Interval: 200 * time.Millisecond})
	subs := p.Subscriptions()
	assert.Len(t, subs, 1)
	assert.Equal(t, 200*time.Millisecond, subs[0].Interval)
	p.Stop()
}
