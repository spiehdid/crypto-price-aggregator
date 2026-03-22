package router_test

import (
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port/mocks"
	"github.com/spiehdid/crypto-price-aggregator/internal/router"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

func newRRProviderState(ctrl *gomock.Controller, name string) *router.ProviderState {
	p := mocks.NewMockPriceProvider(ctrl)
	p.EXPECT().Name().Return(name).AnyTimes()
	return &router.ProviderState{
		Provider: p, Tier: model.TierFree,
		RateLimit:      router.NewRateLimitTracker(30, time.Minute),
		CircuitBreaker: router.NewCircuitBreaker(5, time.Minute),
		AvgLatency:     router.NewLatencyTracker(), Enabled: true,
	}
}

func TestRoundRobin_CyclesThroughProviders(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := newRRProviderState(ctrl, "a")
	b := newRRProviderState(ctrl, "b")
	c := newRRProviderState(ctrl, "c")
	s := router.NewRoundRobinStrategy(true)
	providers := []*router.ProviderState{a, b, c}
	r1, _ := s.Select(providers)
	r2, _ := s.Select(providers)
	r3, _ := s.Select(providers)
	r4, _ := s.Select(providers)
	assert.Equal(t, "a", r1.Provider.Name())
	assert.Equal(t, "b", r2.Provider.Name())
	assert.Equal(t, "c", r3.Provider.Name())
	assert.Equal(t, "a", r4.Provider.Name())
}

func TestRoundRobin_SkipsUnavailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := newRRProviderState(ctrl, "a")
	b := newRRProviderState(ctrl, "b")
	b.Enabled = false
	c := newRRProviderState(ctrl, "c")
	s := router.NewRoundRobinStrategy(true)
	providers := []*router.ProviderState{a, b, c}
	r1, _ := s.Select(providers)
	r2, _ := s.Select(providers)
	r3, _ := s.Select(providers)
	assert.Equal(t, "a", r1.Provider.Name())
	assert.Equal(t, "c", r2.Provider.Name())
	assert.Equal(t, "a", r3.Provider.Name())
}
