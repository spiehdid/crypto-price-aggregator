package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestSubscriptionService_Subscribe(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)

	price := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(67000), Provider: "mock",
		Timestamp: time.Now(), ReceivedAt: time.Now(),
	}
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(price, nil).AnyTimes()

	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router: router, Cache: cache, CacheTTL: 30 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subSvc := app.NewSubscriptionService(ctx, priceSvc, 10*time.Millisecond)

	err := subSvc.Subscribe(ctx, "bitcoin", "usd", 50*time.Millisecond)
	require.NoError(t, err)

	subs := subSvc.Subscriptions()
	assert.Len(t, subs, 1)
	assert.Equal(t, "bitcoin", subs[0].CoinID)
	subSvc.Stop()
}

func TestSubscriptionService_Unsubscribe(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)

	cache.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	router.EXPECT().Select(gomock.Any(), gomock.Any()).Return(nil, model.ErrNoHealthyProvider).AnyTimes()

	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router: router, Cache: cache, CacheTTL: 30 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subSvc := app.NewSubscriptionService(ctx, priceSvc, 10*time.Millisecond)

	_ = subSvc.Subscribe(ctx, "bitcoin", "usd", 1*time.Second)
	err := subSvc.Unsubscribe("bitcoin", "usd")
	require.NoError(t, err)

	subs := subSvc.Subscriptions()
	assert.Len(t, subs, 0)
	subSvc.Stop()
}
