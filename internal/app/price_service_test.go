package app_test

import (
	"context"
	"sync"
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

func newTestPrice() *model.Price {
	return &model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(67000),
		Provider:   "coingecko",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}
}

func TestGetPrice_CacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)

	cached := newTestPrice()
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(cached, nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Cache:    cache,
		CacheTTL: 30 * time.Second,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, cached.Value.Equal(price.Value))
}

func TestGetPrice_CacheMiss_ProviderSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	store := mocks.NewMockPriceStore(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	provider := mocks.NewMockPriceProvider(ctrl)

	expected := newTestPrice()

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil)
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(expected, nil)
	provider.EXPECT().Name().Return("coingecko")
	router.EXPECT().ReportSuccess("coingecko", gomock.Any())
	cache.EXPECT().Set(gomock.Any(), expected, 30*time.Second).Return(nil)
	store.EXPECT().Save(gomock.Any(), expected).Return(nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Cache:    cache,
		Store:    store,
		CacheTTL: 30 * time.Second,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, expected.Value.Equal(price.Value))
}

func TestGetPrice_ProviderFails_FallbackToNext(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	store := mocks.NewMockPriceStore(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	provider1 := mocks.NewMockPriceProvider(ctrl)
	provider2 := mocks.NewMockPriceProvider(ctrl)

	expected := newTestPrice()

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)

	gomock.InOrder(
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider1, nil),
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider2, nil),
	)
	provider1.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(nil, model.ErrRateLimited)
	provider1.EXPECT().Name().Return("provider1")
	router.EXPECT().ReportFailure("provider1", model.ErrRateLimited)

	provider2.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(expected, nil)
	provider2.EXPECT().Name().Return("provider2")
	router.EXPECT().ReportSuccess("provider2", gomock.Any())
	cache.EXPECT().Set(gomock.Any(), expected, 30*time.Second).Return(nil)
	store.EXPECT().Save(gomock.Any(), expected).Return(nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		Cache:      cache,
		Store:      store,
		CacheTTL:   30 * time.Second,
		MaxRetries: 3,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, expected.Value.Equal(price.Value))
}

func TestGetPrice_NoCacheNoStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	provider := mocks.NewMockPriceProvider(ctrl)

	expected := newTestPrice()

	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil)
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(expected, nil)
	provider.EXPECT().Name().Return("coingecko")
	router.EXPECT().ReportSuccess("coingecko", gomock.Any())

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		CacheTTL: 30 * time.Second,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, expected.Value.Equal(price.Value))
}

type mockListener struct {
	mu        sync.Mutex
	lastPrice *model.Price
	done      chan struct{}
}

func newMockListener() *mockListener {
	return &mockListener{done: make(chan struct{}, 1)}
}

func (m *mockListener) OnPriceUpdate(price *model.Price) {
	m.mu.Lock()
	m.lastPrice = price
	m.mu.Unlock()
	select {
	case m.done <- struct{}{}:
	default:
	}
}

func (m *mockListener) wait(t *testing.T) {
	t.Helper()
	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for listener notification")
	}
}

func TestGetPrice_NotifiesListener(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	provider := mocks.NewMockPriceProvider(ctrl)
	listener := newMockListener()

	expected := newTestPrice()
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil)
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(expected, nil)
	provider.EXPECT().Name().Return("coingecko")
	router.EXPECT().ReportSuccess("coingecko", gomock.Any())

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		CacheTTL:   30 * time.Second,
		MaxRetries: 1,
		Listener:   listener,
	})

	_, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	listener.wait(t)
	listener.mu.Lock()
	defer listener.mu.Unlock()
	require.NotNil(t, listener.lastPrice)
	assert.Equal(t, "bitcoin", listener.lastPrice.CoinID)
}

func TestGetPrice_AllProvidersFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(nil, model.ErrNoHealthyProvider)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		Cache:      cache,
		MaxRetries: 3,
	})

	_, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	assert.ErrorIs(t, err, model.ErrNoHealthyProvider)
}

func TestGetPrice_ConsensusMode(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)
	store := mocks.NewMockPriceStore(ctrl)

	p1 := mocks.NewMockPriceProvider(ctrl)
	p2 := mocks.NewMockPriceProvider(ctrl)
	p3 := mocks.NewMockPriceProvider(ctrl)

	price1 := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(67000), Provider: "p1", Timestamp: time.Now(), ReceivedAt: time.Now()}
	price2 := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(67100), Provider: "p2", Timestamp: time.Now(), ReceivedAt: time.Now()}
	price3 := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(67050), Provider: "p3", Timestamp: time.Now(), ReceivedAt: time.Now()}

	// cache miss
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)

	// Consensus engine calls router.Select up to maxProviders (5) times.
	// Return three distinct providers then error to stop iteration.
	p1.EXPECT().Name().Return("p1").AnyTimes()
	p2.EXPECT().Name().Return("p2").AnyTimes()
	p3.EXPECT().Name().Return("p3").AnyTimes()

	gomock.InOrder(
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p1, nil),
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p2, nil),
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p3, nil),
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(nil, model.ErrNoHealthyProvider),
	)

	p1.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(price1, nil)
	p2.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(price2, nil)
	p3.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(price3, nil)

	router.EXPECT().ReportSuccess("p1", gomock.Any())
	router.EXPECT().ReportSuccess("p2", gomock.Any())
	router.EXPECT().ReportSuccess("p3", gomock.Any())

	cache.EXPECT().Set(gomock.Any(), gomock.Any(), 30*time.Second).Return(nil)
	store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	consensus := app.NewConsensusEngine(app.ConsensusConfig{
		Enabled:      true,
		MinProviders: 3,
		MaxDeviation: 5.0,
		Timeout:      5 * time.Second,
	})

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:       router,
		Cache:        cache,
		Store:        store,
		CacheTTL:     30 * time.Second,
		Consensus:    consensus,
		ResponseMode: "consensus",
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	require.NotNil(t, price)
	assert.Equal(t, "consensus", price.Provider)
}

func TestGetPrice_StaleFallbackFromStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)
	store := mocks.NewMockPriceStore(ctrl)

	stale := &model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(65000),
		Provider:   "coingecko",
		Timestamp:  time.Now().Add(-10 * time.Minute),
		ReceivedAt: time.Now().Add(-10 * time.Minute),
	}

	// cache miss
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)
	// all providers fail — router returns error immediately
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(nil, model.ErrNoHealthyProvider)
	// store has stale data
	store.EXPECT().GetLatest(gomock.Any(), "bitcoin", "usd").Return(stale, nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		Cache:      cache,
		Store:      store,
		CacheTTL:   30 * time.Second,
		MaxRetries: 1,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	require.NotNil(t, price)
	assert.True(t, decimal.NewFromFloat(65000).Equal(price.Value))
}
