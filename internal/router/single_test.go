package router_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port/mocks"
	"github.com/spiehdid/crypto-price-aggregator/internal/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestSingleRouter_Select_ReturnsProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	provider := mocks.NewMockPriceProvider(ctrl)
	provider.EXPECT().Name().Return("coingecko").AnyTimes()
	provider.EXPECT().Status().Return(&model.ProviderStatus{
		Name: "coingecko", Healthy: true,
	}).AnyTimes()

	r := router.NewSingleRouter(provider)

	got, err := r.Select(context.Background(), "bitcoin")
	require.NoError(t, err)
	assert.Equal(t, "coingecko", got.Name())
}

func TestSingleRouter_Select_UnhealthyProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	provider := mocks.NewMockPriceProvider(ctrl)
	provider.EXPECT().Name().Return("coingecko").AnyTimes()

	r := router.NewSingleRouter(provider)

	for i := 0; i < 5; i++ {
		r.ReportFailure("coingecko", errors.New("timeout"))
	}

	_, err := r.Select(context.Background(), "bitcoin")
	assert.ErrorIs(t, err, model.ErrNoHealthyProvider)
}

func TestSingleRouter_ReportSuccess_ResetsFailCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	provider := mocks.NewMockPriceProvider(ctrl)
	provider.EXPECT().Name().Return("coingecko").AnyTimes()
	provider.EXPECT().Status().Return(&model.ProviderStatus{
		Name: "coingecko", Healthy: true,
	}).AnyTimes()

	r := router.NewSingleRouter(provider)

	r.ReportFailure("coingecko", errors.New("timeout"))
	r.ReportFailure("coingecko", errors.New("timeout"))
	r.ReportSuccess("coingecko", 100*time.Millisecond)

	got, err := r.Select(context.Background(), "bitcoin")
	require.NoError(t, err)
	assert.Equal(t, "coingecko", got.Name())
}
