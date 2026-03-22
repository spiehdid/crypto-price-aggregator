package router_test

import (
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port/mocks"
	"github.com/spiehdid/crypto-price-aggregator/internal/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

func newPriorityProviderState(ctrl *gomock.Controller, name string, priority int) *router.ProviderState {
	p := mocks.NewMockPriceProvider(ctrl)
	p.EXPECT().Name().Return(name).AnyTimes()
	return &router.ProviderState{
		Provider: p, Tier: model.TierFree, Priority: priority,
		RateLimit:      router.NewRateLimitTracker(30, time.Minute),
		CircuitBreaker: router.NewCircuitBreaker(5, time.Minute),
		AvgLatency:     router.NewLatencyTracker(), Enabled: true,
	}
}

func TestPriorityStrategy_SelectsHighestPriority(t *testing.T) {
	ctrl := gomock.NewController(t)
	p1 := newPriorityProviderState(ctrl, "first", 1)
	p2 := newPriorityProviderState(ctrl, "second", 2)
	p3 := newPriorityProviderState(ctrl, "third", 3)
	s := router.NewPriorityStrategy(true)
	selected, err := s.Select([]*router.ProviderState{p3, p1, p2})
	require.NoError(t, err)
	assert.Equal(t, "first", selected.Provider.Name())
}

func TestPriorityStrategy_SkipsUnavailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	p1 := newPriorityProviderState(ctrl, "first", 1)
	p1.Enabled = false
	p2 := newPriorityProviderState(ctrl, "second", 2)
	s := router.NewPriorityStrategy(true)
	selected, err := s.Select([]*router.ProviderState{p1, p2})
	require.NoError(t, err)
	assert.Equal(t, "second", selected.Provider.Name())
}
