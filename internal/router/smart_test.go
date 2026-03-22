package router_test

import (
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

func newProviderState(ctrl *gomock.Controller, name string, remaining int, latencyMs float64, tier model.ProviderTier) *router.ProviderState {
	p := mocks.NewMockPriceProvider(ctrl)
	p.EXPECT().Name().Return(name).AnyTimes()
	rl := router.NewRateLimitTracker(30, time.Minute)
	for i := 0; i < 30-remaining; i++ {
		rl.Consume()
	}
	lt := router.NewLatencyTracker()
	lt.Record(latencyMs)
	return &router.ProviderState{
		Provider: p, Tier: tier, RateLimit: rl,
		CircuitBreaker: router.NewCircuitBreaker(5, time.Minute),
		AvgLatency:     lt, CostPerCall: decimal.Zero, Enabled: true,
	}
}

func TestSmartStrategy_SelectsBestScore(t *testing.T) {
	ctrl := gomock.NewController(t)
	fast := newProviderState(ctrl, "fast", 20, 50, model.TierFree)
	slow := newProviderState(ctrl, "slow", 20, 200, model.TierFree)
	s := router.NewSmartStrategy(router.SmartWeights{RateRemaining: 0.4, Latency: 0.3, Cost: 0.3}, true)
	selected, err := s.Select([]*router.ProviderState{slow, fast})
	require.NoError(t, err)
	assert.Equal(t, "fast", selected.Provider.Name())
}

func TestSmartStrategy_PrefersMoreRemainingRate(t *testing.T) {
	ctrl := gomock.NewController(t)
	plenty := newProviderState(ctrl, "plenty", 25, 100, model.TierFree)
	scarce := newProviderState(ctrl, "scarce", 2, 100, model.TierFree)
	s := router.NewSmartStrategy(router.SmartWeights{RateRemaining: 0.4, Latency: 0.3, Cost: 0.3}, true)
	selected, err := s.Select([]*router.ProviderState{scarce, plenty})
	require.NoError(t, err)
	assert.Equal(t, "plenty", selected.Provider.Name())
}

func TestSmartStrategy_ExcludesPaidWhenDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	free := newProviderState(ctrl, "free", 20, 100, model.TierFree)
	paid := newProviderState(ctrl, "paid", 20, 50, model.TierPaid)
	s := router.NewSmartStrategy(router.SmartWeights{RateRemaining: 0.4, Latency: 0.3, Cost: 0.3}, false)
	selected, err := s.Select([]*router.ProviderState{paid, free})
	require.NoError(t, err)
	assert.Equal(t, "free", selected.Provider.Name())
}

func TestSmartStrategy_NoAvailableProviders(t *testing.T) {
	s := router.NewSmartStrategy(router.SmartWeights{RateRemaining: 0.4, Latency: 0.3, Cost: 0.3}, true)
	_, err := s.Select([]*router.ProviderState{})
	assert.ErrorIs(t, err, model.ErrNoHealthyProvider)
}
