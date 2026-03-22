package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port/mocks"
	"github.com/spiehdid/crypto-price-aggregator/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// ─── 1. Full retry escalation with different errors ─────────────────────────

func TestGetPrice_RetryEscalation_DifferentErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	store := mocks.NewMockPriceStore(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	p1 := mocks.NewMockPriceProvider(ctrl)
	p2 := mocks.NewMockPriceProvider(ctrl)
	p3 := mocks.NewMockPriceProvider(ctrl)

	expected := &model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(67000),
		Provider:   "binance",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}

	// Cache miss
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)

	// Provider 1: rate limited
	gomock.InOrder(
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p1, nil),
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p2, nil),
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p3, nil),
	)

	p1.EXPECT().Name().Return("coingecko")
	p1.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(nil, model.ErrRateLimited)
	router.EXPECT().ReportFailure("coingecko", model.ErrRateLimited)

	// Provider 2: provider down (timeout)
	p2.EXPECT().Name().Return("coinmarketcap")
	p2.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(nil, model.ErrProviderDown)
	router.EXPECT().ReportFailure("coinmarketcap", model.ErrProviderDown)

	// Provider 3: succeeds
	p3.EXPECT().Name().Return("binance")
	p3.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(expected, nil)
	router.EXPECT().ReportSuccess("binance", gomock.Any())
	cache.EXPECT().Set(gomock.Any(), expected, 30*time.Second).Return(nil)
	store.EXPECT().Save(gomock.Any(), expected).Return(nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		Cache:      cache,
		Store:      store,
		CacheTTL:   30 * time.Second,
		MaxRetries: 5,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, expected.Value.Equal(price.Value))
	assert.Equal(t, "binance", price.Provider)
}

// ─── 2. Retry exhaustion with stale store fallback ──────────────────────────

func TestGetPrice_AllFail_StaleStoreReturnsData(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	store := mocks.NewMockPriceStore(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	p1 := mocks.NewMockPriceProvider(ctrl)
	p2 := mocks.NewMockPriceProvider(ctrl)
	p3 := mocks.NewMockPriceProvider(ctrl)

	stalePrice := &model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(66500),
		Provider:   "coingecko",
		Timestamp:  time.Now().Add(-2 * time.Minute),
		ReceivedAt: time.Now().Add(-2 * time.Minute),
	}

	// Cache miss
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)

	// All 3 providers fail with different errors
	gomock.InOrder(
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p1, nil),
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p2, nil),
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p3, nil),
	)

	p1.EXPECT().Name().Return("coingecko")
	p1.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(nil, model.ErrRateLimited)
	router.EXPECT().ReportFailure("coingecko", model.ErrRateLimited)

	p2.EXPECT().Name().Return("coinmarketcap")
	p2.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(nil, model.ErrProviderDown)
	router.EXPECT().ReportFailure("coinmarketcap", model.ErrProviderDown)

	p3.EXPECT().Name().Return("binance")
	p3.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(nil, model.ErrUnauthorized)
	router.EXPECT().ReportFailure("binance", model.ErrUnauthorized)

	// Store has stale data from 2 minutes ago
	store.EXPECT().GetLatest(gomock.Any(), "bitcoin", "usd").Return(stalePrice, nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		Cache:      cache,
		Store:      store,
		CacheTTL:   30 * time.Second,
		MaxRetries: 3,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err, "should return stale price instead of error")
	require.NotNil(t, price)
	assert.True(t, decimal.NewFromFloat(66500).Equal(price.Value))
	// Verify it IS the stale price (2 min old)
	assert.WithinDuration(t, time.Now().Add(-2*time.Minute), price.ReceivedAt, 5*time.Second)
}

// ─── 3. Retry exhaustion — no store, no cache ───────────────────────────────

func TestGetPrice_AllFail_NoStoreNoCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	p1 := mocks.NewMockPriceProvider(ctrl)
	p2 := mocks.NewMockPriceProvider(ctrl)

	gomock.InOrder(
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p1, nil),
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p2, nil),
	)

	p1.EXPECT().Name().Return("coingecko")
	p1.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(nil, model.ErrRateLimited)
	router.EXPECT().ReportFailure("coingecko", model.ErrRateLimited)

	p2.EXPECT().Name().Return("binance")
	p2.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(nil, model.ErrProviderDown)
	router.EXPECT().ReportFailure("binance", model.ErrProviderDown)

	// No cache, no store — both nil
	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		MaxRetries: 2,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	assert.Nil(t, price, "should not return a price")
	require.Error(t, err, "must return an error, not nil pointer panic")
	// Should return the LAST provider's error
	assert.ErrorIs(t, err, model.ErrProviderDown)
}

// ─── 4. Validator rejects price, falls to next provider ─────────────────────

func TestGetPrice_ValidatorRejectsZeroPrice_FallsToNext(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	p1 := mocks.NewMockPriceProvider(ctrl)
	p2 := mocks.NewMockPriceProvider(ctrl)

	badPrice := &model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.Zero, // zero price — validator should reject
		Provider:   "broken-provider",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}
	goodPrice := &model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(67000),
		Provider:   "reliable-provider",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}

	gomock.InOrder(
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p1, nil),
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p2, nil),
	)

	// Provider 1 returns zero price (no error from provider itself)
	p1.EXPECT().Name().Return("broken-provider")
	p1.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(badPrice, nil)
	// ReportSuccess is called because the provider itself didn't error
	router.EXPECT().ReportSuccess("broken-provider", gomock.Any())
	// Validator rejects → ReportFailure with validation error
	router.EXPECT().ReportFailure("broken-provider", app.ErrPriceZero)

	// Provider 2 returns valid price
	p2.EXPECT().Name().Return("reliable-provider")
	p2.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(goodPrice, nil)
	router.EXPECT().ReportSuccess("reliable-provider", gomock.Any())

	validator := app.NewPriceValidator(app.PriceValidatorConfig{Enabled: true})
	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		Validator:  validator,
		MaxRetries: 3,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, goodPrice.Value.Equal(price.Value), "should use provider 2's price")
	assert.Equal(t, "reliable-provider", price.Provider)
}

// ─── 5. Consensus mode with duplicate provider prevention ───────────────────

func TestGetPrice_Consensus_DeduplicatesProviders(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)

	p1 := mocks.NewMockPriceProvider(ctrl)
	price1 := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(67000), Provider: "only-provider",
		Timestamp: time.Now(), ReceivedAt: time.Now(),
	}

	// Cache miss
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)

	// Router returns the SAME provider every time (SmartStrategy degenerate case)
	p1.EXPECT().Name().Return("only-provider").AnyTimes()

	// First Select returns the provider, subsequent ones return same provider (deduplicated)
	// The consensus engine calls Select up to maxProviders(5) times.
	// First call: new provider. Calls 2-5: same name → skipped via selected map.
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p1, nil).Times(5)

	// Only 1 actual GetPrice call (deduplication)
	p1.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(price1, nil).Times(1)
	router.EXPECT().ReportSuccess("only-provider", gomock.Any()).Times(1)

	// Consensus with MinProviders=3 means 1 price < 3 → confidence=0 → falls through
	// Then single-provider path kicks in
	// After consensus falls through, single-provider path starts
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(p1, nil)
	p1.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(price1, nil)
	router.EXPECT().ReportSuccess("only-provider", gomock.Any())
	cache.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	consensus := app.NewConsensusEngine(app.ConsensusConfig{
		Enabled:      true,
		MinProviders: 3,
		MaxDeviation: 5.0,
		Timeout:      5 * time.Second,
	})

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:       router,
		Cache:        cache,
		CacheTTL:     30 * time.Second,
		Consensus:    consensus,
		ResponseMode: "consensus",
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	require.NotNil(t, price)
	// With only 1 provider, consensus returns Confidence=0, so service falls to single-provider mode
	assert.True(t, decimal.NewFromFloat(67000).Equal(price.Value))
}

// ─── 6. Cache returns error AND non-nil price simultaneously ────────────────

func TestGetPrice_CacheErrorWithNonNilPrice_IgnoresPrice(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)
	provider := mocks.NewMockPriceProvider(ctrl)

	staleCache := &model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(50000), // old cached price
		Provider:   "old",
		Timestamp:  time.Now().Add(-1 * time.Hour),
		ReceivedAt: time.Now().Add(-1 * time.Hour),
	}

	freshPrice := &model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(67000),
		Provider:   "coingecko",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}

	// Cache returns BOTH a price AND an error (e.g., deserialization warning)
	cacheErr := errors.New("partial deserialization error")
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(staleCache, cacheErr)

	// Because err != nil, the code logs warning and falls through (does NOT use cached price)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil)
	provider.EXPECT().Name().Return("coingecko")
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(freshPrice, nil)
	router.EXPECT().ReportSuccess("coingecko", gomock.Any())
	cache.EXPECT().Set(gomock.Any(), freshPrice, gomock.Any()).Return(nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Cache:    cache,
		CacheTTL: 30 * time.Second,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	// Must return fresh price (67000), NOT the stale cache (50000)
	assert.True(t, decimal.NewFromFloat(67000).Equal(price.Value),
		"should ignore cached price when cache returns an error; got %s", price.Value)
}

// ─── 7. Concurrent GetPrice calls don't corrupt counters ────────────────────

func TestGetPrice_ConcurrentCalls_StatsAccurate(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)

	cachedPrice := &model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(67000),
		Provider:   "cached",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}

	// All 100 calls will hit cache
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(cachedPrice, nil).Times(100)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Cache:    cache,
		CacheTTL: 30 * time.Second,
	})

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.GetPrice(context.Background(), "bitcoin", "usd")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d failed", i)
	}

	total, hits, _ := svc.Stats()
	assert.Equal(t, int64(goroutines), total, "requestCount must equal number of goroutines")
	assert.Equal(t, int64(goroutines), hits, "cacheHitCount must equal number of goroutines (all cache hits)")
}

// ─── 8. GetPriceByAddress — registry miss then direct provider learns ───────

func TestGetPriceByAddress_RegistryMiss_DirectProviderLearns(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	tp := mocks.NewMockTokenPriceProvider(ctrl)

	reg := registry.New() // empty — no entries

	tokenPrice := &model.Price{
		CoinID:     "pepe",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(0.00001234),
		Provider:   "coingecko-token",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}

	// Registry has no entry for this address
	// Token provider supports ethereum and returns a price
	tp.EXPECT().SupportsChain("ethereum").Return(true)
	tp.EXPECT().GetPriceByAddress(gomock.Any(), "ethereum", "0xpepeaddr", "usd").Return(tokenPrice, nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:         router,
		Registry:       reg,
		TokenProviders: []port.TokenPriceProvider{tp},
	})

	result, err := svc.GetPriceByAddress(context.Background(), "ethereum", "0xPepeAddr", "usd")
	require.NoError(t, err)

	// Verify: price returned with resolved_via="direct"
	assert.Equal(t, "direct", result.ResolvedVia)
	assert.True(t, decimal.NewFromFloat(0.00001234).Equal(result.Price.Value))

	// Verify: registry now contains the address (self-learning)
	coinID, found := reg.Resolve("ethereum", "0xpepeaddr")
	assert.True(t, found, "registry should have learned the address")
	assert.Equal(t, "pepe", coinID)
}

// ─── 9. Convert with very small prices (memecoin) ───────────────────────────

func TestConvert_SmallPricePrecision(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	// memecoin: 0.000000012345 USD
	memePrice := &model.Price{
		CoinID: "memecoin", Currency: "usd",
		Value:      decimal.RequireFromString("0.000000012345"),
		Provider:   "p",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}
	// bitcoin: 67000 USD
	btcPrice := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value:      decimal.NewFromFloat(67000),
		Provider:   "p",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}

	// memecoin fetch
	memeProvider := mocks.NewMockPriceProvider(ctrl)
	router.EXPECT().Select(gomock.Any(), "memecoin").Return(memeProvider, nil)
	memeProvider.EXPECT().GetPrice(gomock.Any(), "memecoin", "usd").Return(memePrice, nil)
	memeProvider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())

	// bitcoin fetch
	btcProvider := mocks.NewMockPriceProvider(ctrl)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(btcProvider, nil)
	btcProvider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(btcPrice, nil)
	btcProvider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router: router,
	})

	// Convert 1_000_000 memecoin → bitcoin
	amount := decimal.NewFromInt(1_000_000)
	result, err := svc.Convert(context.Background(), "memecoin", "bitcoin", amount, "usd")
	require.NoError(t, err)

	// Expected: 1_000_000 * 0.000000012345 / 67000 ≈ 1.842537e-10
	// rate = memePrice / btcPrice = 0.000000012345 / 67000
	// result = 1_000_000 * rate
	expectedRate := memePrice.Value.Div(btcPrice.Value)
	expectedResult := amount.Mul(expectedRate)

	assert.True(t, expectedResult.Equal(result.Result),
		"expected %s, got %s", expectedResult, result.Result)
	assert.False(t, result.Result.IsZero(), "result must not be zero for small prices")
	assert.True(t, result.Result.IsPositive(), "result must be positive")
	assert.True(t, result.Rate.IsPositive(), "rate must be positive")
}

// ─── 10. Listener notification is async (doesn't block) ─────────────────────

type slowListener struct {
	mu      sync.Mutex
	called  bool
	blockCh chan struct{}
}

func (sl *slowListener) OnPriceUpdate(price *model.Price) {
	// Block for a long time — simulates slow consumer
	<-sl.blockCh
	sl.mu.Lock()
	sl.called = true
	sl.mu.Unlock()
}

func TestGetPrice_SlowListener_DoesNotBlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	provider := mocks.NewMockPriceProvider(ctrl)

	expected := &model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(67000),
		Provider:   "coingecko",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}

	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil)
	provider.EXPECT().Name().Return("coingecko")
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(expected, nil)
	router.EXPECT().ReportSuccess("coingecko", gomock.Any())

	listener := &slowListener{blockCh: make(chan struct{})}

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Listener: listener,
	})

	start := time.Now()
	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.True(t, expected.Value.Equal(price.Value))

	// GetPrice must return quickly — listener is async (goroutine)
	assert.Less(t, elapsed, 500*time.Millisecond,
		"GetPrice should not block on slow listener; took %v", elapsed)

	// Unblock the listener so the goroutine doesn't leak
	close(listener.blockCh)

	// Give goroutine time to finish
	time.Sleep(50 * time.Millisecond)
	listener.mu.Lock()
	assert.True(t, listener.called, "listener should eventually be called")
	listener.mu.Unlock()
}
