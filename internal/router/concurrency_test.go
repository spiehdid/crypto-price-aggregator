package router_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port/mocks"
	"github.com/spiehdid/crypto-price-aggregator/internal/router"
	"go.uber.org/mock/gomock"
)

func TestProviderState_ConcurrentSafety(t *testing.T) {
	ctrl := gomock.NewController(t)
	p := mocks.NewMockPriceProvider(ctrl)
	p.EXPECT().Name().Return("p1").AnyTimes()

	ps := &router.ProviderState{
		Provider:       p,
		Tier:           model.TierFree,
		RateLimit:      router.NewRateLimitTracker(1000, time.Minute),
		CircuitBreaker: router.NewCircuitBreaker(50, time.Minute),
		AvgLatency:     router.NewLatencyTracker(),
		CostPerCall:    decimal.Zero,
		Enabled:        true,
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); ps.RecordAccurate() }()
		go func() { defer wg.Done(); ps.RecordOutlier() }()
		go func() { defer wg.Done(); ps.IsAvailable(true) }()
	}
	wg.Wait()
}

func TestMultiRouter_ConcurrentSelectAndReport(t *testing.T) {
	ctrl := gomock.NewController(t)
	p1 := mocks.NewMockPriceProvider(ctrl)
	p1.EXPECT().Name().Return("p1").AnyTimes()
	p2 := mocks.NewMockPriceProvider(ctrl)
	p2.EXPECT().Name().Return("p2").AnyTimes()

	r := router.NewMultiRouter(
		router.NewSmartStrategy(router.SmartWeights{RateRemaining: 0.4, Latency: 0.3, Cost: 0.3}, true),
		[]router.ProviderRegistration{
			{Provider: p1, Tier: model.TierFree, Priority: 1, RateLimit: 10000, RateWindow: time.Minute, CBThreshold: 50, CBTimeout: time.Minute, CostPerCall: decimal.Zero},
			{Provider: p2, Tier: model.TierFree, Priority: 2, RateLimit: 10000, RateWindow: time.Minute, CBThreshold: 50, CBTimeout: time.Minute, CostPerCall: decimal.Zero},
		},
	)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			_, _ = r.Select(context.Background(), "bitcoin")
		}()
		go func() {
			defer wg.Done()
			r.ReportSuccess("p1", 50*time.Millisecond)
		}()
		go func() {
			defer wg.Done()
			r.ReportAccurate("p2")
		}()
		go func() {
			defer wg.Done()
			r.ReportOutlier("p1")
		}()
	}
	wg.Wait()
}
