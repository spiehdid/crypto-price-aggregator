package router_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port/mocks"
	"github.com/spiehdid/crypto-price-aggregator/internal/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Test 1: All providers exhausted rate limits.
// 3 providers each with rate_limit=1. After 3 successful Select() calls
// (one per provider), the 4th must return ErrNoHealthyProvider.
func TestMultiRouter_AllRateLimitsExhausted(t *testing.T) {
	ctrl := gomock.NewController(t)

	p1 := mocks.NewMockPriceProvider(ctrl)
	p1.EXPECT().Name().Return("p1").AnyTimes()
	p2 := mocks.NewMockPriceProvider(ctrl)
	p2.EXPECT().Name().Return("p2").AnyTimes()
	p3 := mocks.NewMockPriceProvider(ctrl)
	p3.EXPECT().Name().Return("p3").AnyTimes()

	r := router.NewMultiRouter(
		router.NewSmartStrategy(router.SmartWeights{RateRemaining: 1.0, Latency: 0, Cost: 0}, true),
		[]router.ProviderRegistration{
			{Provider: p1, Tier: model.TierFree, Priority: 1, RateLimit: 1, RateWindow: time.Hour, CBThreshold: 100, CBTimeout: time.Hour, CostPerCall: decimal.Zero},
			{Provider: p2, Tier: model.TierFree, Priority: 2, RateLimit: 1, RateWindow: time.Hour, CBThreshold: 100, CBTimeout: time.Hour, CostPerCall: decimal.Zero},
			{Provider: p3, Tier: model.TierFree, Priority: 3, RateLimit: 1, RateWindow: time.Hour, CBThreshold: 100, CBTimeout: time.Hour, CostPerCall: decimal.Zero},
		},
	)

	// First 3 calls should succeed (one token per provider).
	for i := 0; i < 3; i++ {
		selected, err := r.Select(context.Background(), "bitcoin")
		require.NoError(t, err, "call %d should succeed", i+1)
		require.NotNil(t, selected)
	}

	// 4th call: all rate limits exhausted.
	_, err := r.Select(context.Background(), "bitcoin")
	assert.ErrorIs(t, err, model.ErrNoHealthyProvider)
}

// Test 2: Circuit breaker full lifecycle: closed -> open -> half-open -> closed.
func TestMultiRouter_CircuitBreakerLifecycle(t *testing.T) {
	ctrl := gomock.NewController(t)

	p1 := mocks.NewMockPriceProvider(ctrl)
	p1.EXPECT().Name().Return("p1").AnyTimes()
	p2 := mocks.NewMockPriceProvider(ctrl)
	p2.EXPECT().Name().Return("p2").AnyTimes()

	cbTimeout := 80 * time.Millisecond

	r := router.NewMultiRouter(
		router.NewPriorityStrategy(true),
		[]router.ProviderRegistration{
			{Provider: p1, Tier: model.TierFree, Priority: 1, RateLimit: 100, RateWindow: time.Hour, CBThreshold: 2, CBTimeout: cbTimeout, CostPerCall: decimal.Zero},
			{Provider: p2, Tier: model.TierFree, Priority: 2, RateLimit: 100, RateWindow: time.Hour, CBThreshold: 2, CBTimeout: cbTimeout, CostPerCall: decimal.Zero},
		},
	)

	// p1 is highest priority, should be selected first.
	selected, err := r.Select(context.Background(), "bitcoin")
	require.NoError(t, err)
	assert.Equal(t, "p1", selected.Name())

	// 2 failures trip the circuit breaker on p1.
	r.ReportFailure("p1", errors.New("timeout"))
	r.ReportFailure("p1", errors.New("timeout"))

	// CB open: p1 skipped, p2 selected.
	selected, err = r.Select(context.Background(), "bitcoin")
	require.NoError(t, err)
	assert.Equal(t, "p2", selected.Name(), "should fall back to p2 while p1 CB is open")

	// Wait for recovery timeout so CB transitions to half-open.
	time.Sleep(cbTimeout + 20*time.Millisecond)

	// Half-open: p1 should be allowed one probe request.
	selected, err = r.Select(context.Background(), "bitcoin")
	require.NoError(t, err)
	assert.Equal(t, "p1", selected.Name(), "p1 should be tried in half-open state")

	// Report success: CB closes.
	r.ReportSuccess("p1", 10*time.Millisecond)

	// p1 back to normal.
	selected, err = r.Select(context.Background(), "bitcoin")
	require.NoError(t, err)
	assert.Equal(t, "p1", selected.Name(), "p1 should be back after CB closes")
}

// Test 3: Accuracy score affects smart strategy provider selection.
// Provider A has perfect accuracy, Provider B has 50% accuracy.
// Even if B has better latency, A should be preferred because accuracy
// multiplies the base score.
func TestSmartStrategy_AccuracyAffectsSelection(t *testing.T) {
	ctrl := gomock.NewController(t)

	// Provider A: high accuracy, mediocre latency
	pA := mocks.NewMockPriceProvider(ctrl)
	pA.EXPECT().Name().Return("accurate").AnyTimes()
	rlA := router.NewRateLimitTracker(30, time.Minute)
	ltA := router.NewLatencyTracker()
	ltA.Record(200) // 200ms — slower
	psA := &router.ProviderState{
		Provider: pA, Tier: model.TierFree, RateLimit: rlA,
		CircuitBreaker: router.NewCircuitBreaker(5, time.Minute),
		AvgLatency:     ltA, CostPerCall: decimal.Zero, Enabled: true,
	}
	// Record 10 accurate responses -> accuracy = 1.0
	for i := 0; i < 10; i++ {
		psA.RecordAccurate()
	}

	// Provider B: low accuracy, great latency
	pB := mocks.NewMockPriceProvider(ctrl)
	pB.EXPECT().Name().Return("fast_inaccurate").AnyTimes()
	rlB := router.NewRateLimitTracker(30, time.Minute)
	ltB := router.NewLatencyTracker()
	ltB.Record(10) // 10ms — much faster
	psB := &router.ProviderState{
		Provider: pB, Tier: model.TierFree, RateLimit: rlB,
		CircuitBreaker: router.NewCircuitBreaker(5, time.Minute),
		AvgLatency:     ltB, CostPerCall: decimal.Zero, Enabled: true,
	}
	// Record 5 accurate + 5 outlier -> accuracy = 0.5
	for i := 0; i < 5; i++ {
		psB.RecordAccurate()
	}
	for i := 0; i < 5; i++ {
		psB.RecordOutlier()
	}

	// Use weights that heavily favor latency to make the test meaningful:
	// Without accuracy, B would win due to much better latency.
	s := router.NewSmartStrategy(router.SmartWeights{
		RateRemaining: 0.2,
		Latency:       0.7,
		Cost:          0.1,
	}, true)

	selected, err := s.Select([]*router.ProviderState{psB, psA})
	require.NoError(t, err)
	assert.Equal(t, "accurate", selected.Provider.Name(),
		"provider with 1.0 accuracy should beat provider with 0.5 accuracy even if slower")
}

// Test 4: 5 consecutive outliers disable a provider.
func TestProviderState_OutlierThreshold_DisablesProvider(t *testing.T) {
	ctrl := gomock.NewController(t)

	p := mocks.NewMockPriceProvider(ctrl)
	p.EXPECT().Name().Return("outlier_prone").AnyTimes()

	ps := &router.ProviderState{
		Provider:       p,
		Tier:           model.TierFree,
		RateLimit:      router.NewRateLimitTracker(30, time.Minute),
		CircuitBreaker: router.NewCircuitBreaker(100, time.Minute),
		AvgLatency:     router.NewLatencyTracker(),
		CostPerCall:    decimal.Zero,
		Enabled:        true,
	}

	// 4 outliers: still available.
	for i := 0; i < 4; i++ {
		ps.RecordOutlier()
	}
	assert.True(t, ps.IsAvailable(true), "4 outliers should not disable provider")

	// 5th outlier: provider disabled.
	ps.RecordOutlier()
	assert.False(t, ps.IsAvailable(true), "5 consecutive outliers should disable provider")

	// Also verify Select skips this provider.
	strategy := router.NewSmartStrategy(router.SmartWeights{RateRemaining: 1, Latency: 0, Cost: 0}, true)
	_, err := strategy.Select([]*router.ProviderState{ps})
	assert.ErrorIs(t, err, model.ErrNoHealthyProvider, "Select should skip provider disabled by outliers")

	// Accurate response resets outlier count: provider becomes available again.
	ps.RecordAccurate()
	assert.True(t, ps.IsAvailable(true), "accurate response should reset outlier count")
}

// Test 5: Concurrent Select + ReportFailure with race detector.
// 50 goroutines calling Select, 50 calling ReportFailure. Must not panic.
func TestMultiRouter_ConcurrentSelectAndReport_NoRace(t *testing.T) {
	ctrl := gomock.NewController(t)

	p1 := mocks.NewMockPriceProvider(ctrl)
	p1.EXPECT().Name().Return("p1").AnyTimes()
	p2 := mocks.NewMockPriceProvider(ctrl)
	p2.EXPECT().Name().Return("p2").AnyTimes()

	r := router.NewMultiRouter(
		router.NewSmartStrategy(router.SmartWeights{RateRemaining: 0.4, Latency: 0.3, Cost: 0.3}, true),
		[]router.ProviderRegistration{
			{Provider: p1, Tier: model.TierFree, Priority: 1, RateLimit: 10000, RateWindow: time.Hour, CBThreshold: 10000, CBTimeout: time.Hour, CostPerCall: decimal.Zero},
			{Provider: p2, Tier: model.TierFree, Priority: 2, RateLimit: 10000, RateWindow: time.Hour, CBThreshold: 10000, CBTimeout: time.Hour, CostPerCall: decimal.Zero},
		},
	)

	var wg sync.WaitGroup
	var selectCount atomic.Int64
	var failureCount atomic.Int64

	// 50 goroutines calling Select.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.Select(context.Background(), "bitcoin")
			selectCount.Add(1)
		}()
	}

	// 50 goroutines calling ReportFailure.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.ReportFailure("p1", errors.New("timeout"))
			failureCount.Add(1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(50), selectCount.Load(), "all selects should complete")
	assert.Equal(t, int64(50), failureCount.Load(), "all failures should be recorded")
}

// Test 6: Rate limit auto-reset after window expires.
func TestRateLimitTracker_WindowReset(t *testing.T) {
	window := 60 * time.Millisecond
	rl := router.NewRateLimitTracker(3, window)

	// Consume all tokens.
	rl.Consume()
	rl.Consume()
	rl.Consume()
	assert.True(t, rl.IsExhausted(), "should be exhausted after consuming all tokens")
	assert.Equal(t, 0, rl.Remaining())

	// Wait for window to pass.
	time.Sleep(window + 20*time.Millisecond)

	// Tokens should be restored.
	assert.False(t, rl.IsExhausted(), "should not be exhausted after window reset")
	assert.Equal(t, 3, rl.Remaining(), "all tokens should be restored after window")

	// Can consume again.
	rl.Consume()
	assert.Equal(t, 2, rl.Remaining())
}
