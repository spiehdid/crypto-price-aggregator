package router_test

import (
	"context"
	"errors"
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

func TestMultiRouter_SelectsViaStrategy(t *testing.T) {
	ctrl := gomock.NewController(t)
	p1 := mocks.NewMockPriceProvider(ctrl)
	p1.EXPECT().Name().Return("p1").AnyTimes()
	p2 := mocks.NewMockPriceProvider(ctrl)
	p2.EXPECT().Name().Return("p2").AnyTimes()
	r := router.NewMultiRouter(
		router.NewSmartStrategy(router.SmartWeights{RateRemaining: 0.4, Latency: 0.3, Cost: 0.3}, true),
		[]router.ProviderRegistration{
			{Provider: p1, Tier: model.TierFree, Priority: 1, RateLimit: 30, RateWindow: time.Minute, CBThreshold: 5, CBTimeout: time.Minute, CostPerCall: decimal.Zero},
			{Provider: p2, Tier: model.TierFree, Priority: 2, RateLimit: 30, RateWindow: time.Minute, CBThreshold: 5, CBTimeout: time.Minute, CostPerCall: decimal.Zero},
		},
	)
	selected, err := r.Select(context.Background(), "bitcoin")
	require.NoError(t, err)
	assert.NotNil(t, selected)
}

func TestMultiRouter_ReportFailure_AffectsSelection(t *testing.T) {
	ctrl := gomock.NewController(t)
	p1 := mocks.NewMockPriceProvider(ctrl)
	p1.EXPECT().Name().Return("p1").AnyTimes()
	p2 := mocks.NewMockPriceProvider(ctrl)
	p2.EXPECT().Name().Return("p2").AnyTimes()
	r := router.NewMultiRouter(
		router.NewPriorityStrategy(true),
		[]router.ProviderRegistration{
			{Provider: p1, Tier: model.TierFree, Priority: 1, RateLimit: 30, RateWindow: time.Minute, CBThreshold: 2, CBTimeout: time.Minute, CostPerCall: decimal.Zero},
			{Provider: p2, Tier: model.TierFree, Priority: 2, RateLimit: 30, RateWindow: time.Minute, CBThreshold: 2, CBTimeout: time.Minute, CostPerCall: decimal.Zero},
		},
	)
	r.ReportFailure("p1", errors.New("err"))
	r.ReportFailure("p1", errors.New("err"))
	selected, err := r.Select(context.Background(), "bitcoin")
	require.NoError(t, err)
	assert.Equal(t, "p2", selected.Name())
}

func TestMultiRouter_ReportSuccess_RecordsLatency(t *testing.T) {
	ctrl := gomock.NewController(t)
	p1 := mocks.NewMockPriceProvider(ctrl)
	p1.EXPECT().Name().Return("p1").AnyTimes()
	r := router.NewMultiRouter(
		router.NewSmartStrategy(router.SmartWeights{RateRemaining: 0.4, Latency: 0.3, Cost: 0.3}, true),
		[]router.ProviderRegistration{
			{Provider: p1, Tier: model.TierFree, Priority: 1, RateLimit: 30, RateWindow: time.Minute, CBThreshold: 5, CBTimeout: time.Minute, CostPerCall: decimal.Zero},
		},
	)
	r.ReportSuccess("p1", 100*time.Millisecond)
	selected, err := r.Select(context.Background(), "bitcoin")
	require.NoError(t, err)
	assert.Equal(t, "p1", selected.Name())
}
