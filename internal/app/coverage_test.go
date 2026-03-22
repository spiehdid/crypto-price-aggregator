package app_test

import (
	"context"
	"errors"
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

// ─── PriceService: Stats ────────────────────────────────────────────────────

func TestPriceService_Stats_InitialValues(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	svc := app.NewPriceService(app.PriceServiceDeps{Router: router})

	total, hits, uptime := svc.Stats()
	assert.Equal(t, int64(0), total)
	assert.Equal(t, int64(0), hits)
	assert.GreaterOrEqual(t, uptime.Milliseconds(), int64(0))
}

func TestPriceService_Stats_AfterRequests(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)

	price := newTestPrice()
	// First call: cache miss → provider hit
	// Second call: cache hit
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil).Times(1)
	provider := mocks.NewMockPriceProvider(ctrl)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil).Times(1)
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(price, nil).Times(1)
	provider.EXPECT().Name().Return("p1").Times(1)
	router.EXPECT().ReportSuccess("p1", gomock.Any()).Times(1)
	cache.EXPECT().Set(gomock.Any(), price, gomock.Any()).Return(nil).Times(1)

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(price, nil).Times(1)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Cache:    cache,
		CacheTTL: 30 * time.Second,
	})

	_, _ = svc.GetPrice(context.Background(), "bitcoin", "usd")
	_, _ = svc.GetPrice(context.Background(), "bitcoin", "usd")

	total, hits, _ := svc.Stats()
	assert.Equal(t, int64(2), total)
	assert.Equal(t, int64(1), hits)
}

// ─── PriceService: GetOHLC ──────────────────────────────────────────────────

func TestPriceService_GetOHLC_NoStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	svc := app.NewPriceService(app.PriceServiceDeps{Router: router})

	_, err := svc.GetOHLC(context.Background(), "bitcoin", "usd", time.Now().Add(-time.Hour), time.Now(), time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestPriceService_GetOHLC_WithStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	store := mocks.NewMockPriceStore(ctrl)

	from := time.Now().Add(-time.Hour)
	to := time.Now()
	interval := time.Minute

	expected := []model.OHLC{{Open: decimal.NewFromFloat(67000), Close: decimal.NewFromFloat(67500)}}
	store.EXPECT().GetOHLC(gomock.Any(), "bitcoin", "usd", from, to, interval).Return(expected, nil)

	svc := app.NewPriceService(app.PriceServiceDeps{Router: router, Store: store})

	ohlc, err := svc.GetOHLC(context.Background(), "bitcoin", "usd", from, to, interval)
	require.NoError(t, err)
	assert.Len(t, ohlc, 1)
}

// ─── PriceService: Convert ──────────────────────────────────────────────────

func TestPriceService_Convert_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)

	btcPrice := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(60000), Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}
	ethPrice := &model.Price{CoinID: "ethereum", Currency: "usd", Value: decimal.NewFromFloat(3000), Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}

	// bitcoin fetch: cache miss then provider
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)
	btcProvider := mocks.NewMockPriceProvider(ctrl)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(btcProvider, nil)
	btcProvider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(btcPrice, nil)
	btcProvider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())
	cache.EXPECT().Set(gomock.Any(), btcPrice, gomock.Any()).Return(nil)

	// ethereum fetch: cache miss then provider
	cache.EXPECT().Get(gomock.Any(), "ethereum", "usd").Return(nil, nil)
	ethProvider := mocks.NewMockPriceProvider(ctrl)
	router.EXPECT().Select(gomock.Any(), "ethereum").Return(ethProvider, nil)
	ethProvider.EXPECT().GetPrice(gomock.Any(), "ethereum", "usd").Return(ethPrice, nil)
	ethProvider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())
	cache.EXPECT().Set(gomock.Any(), ethPrice, gomock.Any()).Return(nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Cache:    cache,
		CacheTTL: 30 * time.Second,
	})

	result, err := svc.Convert(context.Background(), "bitcoin", "ethereum", decimal.NewFromFloat(1), "usd")
	require.NoError(t, err)
	assert.Equal(t, "bitcoin", result.From)
	assert.Equal(t, "ethereum", result.To)
	// 1 BTC = 60000/3000 = 20 ETH
	assert.True(t, decimal.NewFromFloat(20).Equal(result.Result))
	assert.Equal(t, "usd", result.ViaCurrency)
}

func TestPriceService_Convert_FromPriceFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(nil, model.ErrNoHealthyProvider)

	svc := app.NewPriceService(app.PriceServiceDeps{Router: router})

	_, err := svc.Convert(context.Background(), "bitcoin", "ethereum", decimal.NewFromFloat(1), "usd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bitcoin")
}

func TestPriceService_Convert_ToPriceFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)

	btcPrice := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(60000), Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)
	btcProvider := mocks.NewMockPriceProvider(ctrl)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(btcProvider, nil)
	btcProvider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(btcPrice, nil)
	btcProvider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())
	cache.EXPECT().Set(gomock.Any(), btcPrice, gomock.Any()).Return(nil)

	cache.EXPECT().Get(gomock.Any(), "ethereum", "usd").Return(nil, nil)
	router.EXPECT().Select(gomock.Any(), "ethereum").Return(nil, model.ErrNoHealthyProvider)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Cache:    cache,
		CacheTTL: 30 * time.Second,
	})

	_, err := svc.Convert(context.Background(), "bitcoin", "ethereum", decimal.NewFromFloat(1), "usd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ethereum")
}

func TestPriceService_Convert_ToPriceZero(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	btcPrice := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(60000), Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}
	ethPrice := &model.Price{CoinID: "ethereum", Currency: "usd", Value: decimal.Zero, Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}

	btcProvider := mocks.NewMockPriceProvider(ctrl)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(btcProvider, nil)
	btcProvider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(btcPrice, nil)
	btcProvider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())

	ethProvider := mocks.NewMockPriceProvider(ctrl)
	router.EXPECT().Select(gomock.Any(), "ethereum").Return(ethProvider, nil)
	ethProvider.EXPECT().GetPrice(gomock.Any(), "ethereum", "usd").Return(ethPrice, nil)
	ethProvider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())

	svc := app.NewPriceService(app.PriceServiceDeps{Router: router})

	_, err := svc.Convert(context.Background(), "bitcoin", "ethereum", decimal.NewFromFloat(1), "usd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "zero")
}

// ─── PriceService: GetPriceByAddress ────────────────────────────────────────

func TestPriceService_GetPriceByAddress_RegistryHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	reg := registry.New()
	reg.Add("ethereum", "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", "usd-coin")

	expectedPrice := &model.Price{CoinID: "usd-coin", Currency: "usd", Value: decimal.NewFromFloat(1.0), Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}

	provider := mocks.NewMockPriceProvider(ctrl)
	router.EXPECT().Select(gomock.Any(), "usd-coin").Return(provider, nil)
	provider.EXPECT().GetPrice(gomock.Any(), "usd-coin", "usd").Return(expectedPrice, nil)
	provider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Registry: reg,
	})

	result, err := svc.GetPriceByAddress(context.Background(), "ethereum", "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", "usd")
	require.NoError(t, err)
	assert.Equal(t, "registry", result.ResolvedVia)
	assert.True(t, decimal.NewFromFloat(1.0).Equal(result.Price.Value))
}

func TestPriceService_GetPriceByAddress_RegistryHit_AddressUppercase(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	reg := registry.New()
	// Add with lowercase, resolve with uppercase — should still match
	reg.Add("ethereum", "0xtoken", "my-token")

	expectedPrice := &model.Price{CoinID: "my-token", Currency: "usd", Value: decimal.NewFromFloat(5.0), Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}

	provider := mocks.NewMockPriceProvider(ctrl)
	router.EXPECT().Select(gomock.Any(), "my-token").Return(provider, nil)
	provider.EXPECT().GetPrice(gomock.Any(), "my-token", "usd").Return(expectedPrice, nil)
	provider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Registry: reg,
	})

	// Pass uppercase — should be lowercased internally
	result, err := svc.GetPriceByAddress(context.Background(), "ETHEREUM", "0XTOKEN", "usd")
	require.NoError(t, err)
	assert.Equal(t, "registry", result.ResolvedVia)
}

func TestPriceService_GetPriceByAddress_TokenProvider_Direct(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	tp := mocks.NewMockTokenPriceProvider(ctrl)

	tokenPrice := &model.Price{CoinID: "shib", Currency: "usd", Value: decimal.NewFromFloat(0.00001), Provider: "tp", Timestamp: time.Now(), ReceivedAt: time.Now()}

	tp.EXPECT().SupportsChain("ethereum").Return(true)
	tp.EXPECT().GetPriceByAddress(gomock.Any(), "ethereum", "0xshib", "usd").Return(tokenPrice, nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:         router,
		TokenProviders: []port.TokenPriceProvider{tp},
	})

	result, err := svc.GetPriceByAddress(context.Background(), "ethereum", "0xshib", "usd")
	require.NoError(t, err)
	assert.Equal(t, "direct", result.ResolvedVia)
	assert.True(t, decimal.NewFromFloat(0.00001).Equal(result.Price.Value))
}

func TestPriceService_GetPriceByAddress_TokenProvider_DirectWithRegistry(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	tp := mocks.NewMockTokenPriceProvider(ctrl)

	reg := registry.New() // empty registry — no hit

	tokenPrice := &model.Price{CoinID: "shib", Currency: "usd", Value: decimal.NewFromFloat(0.00001), Provider: "tp", Timestamp: time.Now(), ReceivedAt: time.Now()}

	tp.EXPECT().SupportsChain("ethereum").Return(true)
	tp.EXPECT().GetPriceByAddress(gomock.Any(), "ethereum", "0xshib", "usd").Return(tokenPrice, nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:         router,
		Registry:       reg,
		TokenProviders: []port.TokenPriceProvider{tp},
	})

	result, err := svc.GetPriceByAddress(context.Background(), "ethereum", "0xshib", "usd")
	require.NoError(t, err)
	assert.Equal(t, "direct", result.ResolvedVia)

	// Should also have added to registry for future lookups
	coinID, found := reg.Resolve("ethereum", "0xshib")
	assert.True(t, found)
	assert.Equal(t, "shib", coinID)
}

func TestPriceService_GetPriceByAddress_TokenProvider_ChainNotSupported(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	tp := mocks.NewMockTokenPriceProvider(ctrl)

	tp.EXPECT().SupportsChain("solana").Return(false)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:         router,
		TokenProviders: []port.TokenPriceProvider{tp},
	})

	_, err := svc.GetPriceByAddress(context.Background(), "solana", "0xtoken", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestPriceService_GetPriceByAddress_TokenProvider_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	tp := mocks.NewMockTokenPriceProvider(ctrl)

	tp.EXPECT().SupportsChain("ethereum").Return(true)
	tp.EXPECT().GetPriceByAddress(gomock.Any(), "ethereum", "0xtoken", "usd").Return(nil, errors.New("upstream error"))

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:         router,
		TokenProviders: []port.TokenPriceProvider{tp},
	})

	_, err := svc.GetPriceByAddress(context.Background(), "ethereum", "0xtoken", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestPriceService_GetPriceByAddress_NoRegistry_NoProviders(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	svc := app.NewPriceService(app.PriceServiceDeps{Router: router})

	_, err := svc.GetPriceByAddress(context.Background(), "ethereum", "0xtoken", "usd")
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

// ─── PriceService: Validator integration ───────────────────────────────────

func TestPriceService_Validator_RejectsZeroPrice_FallsToNextProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	provider1 := mocks.NewMockPriceProvider(ctrl)
	provider2 := mocks.NewMockPriceProvider(ctrl)

	badPrice := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.Zero, Provider: "p1", Timestamp: time.Now(), ReceivedAt: time.Now()}
	goodPrice := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(67000), Provider: "p2", Timestamp: time.Now(), ReceivedAt: time.Now()}

	gomock.InOrder(
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider1, nil),
		router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider2, nil),
	)
	provider1.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(badPrice, nil)
	// Name() is called once for provider1 (stored in local var, reused for ReportSuccess, slog.Warn, ReportFailure)
	provider1.EXPECT().Name().Return("p1").Times(1)
	router.EXPECT().ReportSuccess("p1", gomock.Any())
	router.EXPECT().ReportFailure("p1", app.ErrPriceZero)

	provider2.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(goodPrice, nil)
	provider2.EXPECT().Name().Return("p2")
	router.EXPECT().ReportSuccess("p2", gomock.Any())

	validator := app.NewPriceValidator(app.PriceValidatorConfig{Enabled: true})
	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		Validator:  validator,
		MaxRetries: 3,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, decimal.NewFromFloat(67000).Equal(price.Value))
}

func TestPriceService_Validator_ValidatesAgainstCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)
	provider := mocks.NewMockPriceProvider(ctrl)

	cachedRef := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(67000), Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}
	newPrice := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(80000), Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}

	// Primary cache miss
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil)
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(newPrice, nil)
	// Name() is called once (stored in local var, reused for ReportSuccess and slog.Warn)
	provider.EXPECT().Name().Return("p").Times(1)
	router.EXPECT().ReportSuccess("p", gomock.Any())
	// Validator reads reference from cache
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(cachedRef, nil)
	// Cache set proceeds even with deviation warning (non-fatal)
	cache.EXPECT().Set(gomock.Any(), newPrice, gomock.Any()).Return(nil)

	validator := app.NewPriceValidator(app.PriceValidatorConfig{
		Enabled:               true,
		MaxDeviationFromCache: 5.0, // 19% deviation will warn but not fail
	})
	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:    router,
		Cache:     cache,
		CacheTTL:  30 * time.Second,
		Validator: validator,
	})

	// Should succeed despite high deviation (non-fatal)
	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, decimal.NewFromFloat(80000).Equal(price.Value))
}

// ─── PriceService: GetPrices ─────────────────────────────────────────────────

func TestPriceService_GetPrices_PartialFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	btcPrice := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(60000), Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}

	btcProvider := mocks.NewMockPriceProvider(ctrl)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(btcProvider, nil)
	btcProvider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(btcPrice, nil)
	btcProvider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())

	router.EXPECT().Select(gomock.Any(), "ethereum").Return(nil, model.ErrNoHealthyProvider)

	svc := app.NewPriceService(app.PriceServiceDeps{Router: router})

	prices, errs := svc.GetPrices(context.Background(), []string{"bitcoin", "ethereum"}, "usd")
	assert.Len(t, prices, 1)
	assert.Equal(t, "bitcoin", prices[0].CoinID)
	assert.Contains(t, errs, "ethereum")
	assert.ErrorIs(t, errs["ethereum"], model.ErrNoHealthyProvider)
}

func TestPriceService_GetPrices_AllSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	btcPrice := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(60000), Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}
	ethPrice := &model.Price{CoinID: "ethereum", Currency: "usd", Value: decimal.NewFromFloat(3000), Provider: "p", Timestamp: time.Now(), ReceivedAt: time.Now()}

	pBTC := mocks.NewMockPriceProvider(ctrl)
	pETH := mocks.NewMockPriceProvider(ctrl)

	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(pBTC, nil)
	pBTC.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(btcPrice, nil)
	pBTC.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())

	router.EXPECT().Select(gomock.Any(), "ethereum").Return(pETH, nil)
	pETH.EXPECT().GetPrice(gomock.Any(), "ethereum", "usd").Return(ethPrice, nil)
	pETH.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())

	svc := app.NewPriceService(app.PriceServiceDeps{Router: router})

	prices, errs := svc.GetPrices(context.Background(), []string{"bitcoin", "ethereum"}, "usd")
	assert.Len(t, prices, 2)
	assert.Empty(t, errs)
}

// ─── PriceService: cache error on Get ───────────────────────────────────────

func TestPriceService_CacheGetError_FallsThrough(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)
	provider := mocks.NewMockPriceProvider(ctrl)

	expected := newTestPrice()
	cacheErr := errors.New("redis connection error")

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, cacheErr)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil)
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(expected, nil)
	provider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())
	cache.EXPECT().Set(gomock.Any(), expected, gomock.Any()).Return(nil)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Cache:    cache,
		CacheTTL: 30 * time.Second,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, expected.Value.Equal(price.Value))
}

// ─── PriceService: cache set error ──────────────────────────────────────────

func TestPriceService_CacheSetError_DoesNotFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)
	provider := mocks.NewMockPriceProvider(ctrl)

	expected := newTestPrice()

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil)
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(expected, nil)
	provider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())
	cache.EXPECT().Set(gomock.Any(), expected, gomock.Any()).Return(errors.New("cache full"))

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Cache:    cache,
		CacheTTL: 30 * time.Second,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, expected.Value.Equal(price.Value))
}

// ─── PriceService: store save error ─────────────────────────────────────────

func TestPriceService_StoreSaveError_DoesNotFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	store := mocks.NewMockPriceStore(ctrl)
	provider := mocks.NewMockPriceProvider(ctrl)

	expected := newTestPrice()

	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil)
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(expected, nil)
	provider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())
	store.EXPECT().Save(gomock.Any(), expected).Return(errors.New("db error"))

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router: router,
		Store:  store,
	})

	price, err := svc.GetPrice(context.Background(), "bitcoin", "usd")
	require.NoError(t, err)
	assert.True(t, expected.Value.Equal(price.Value))
}

// ─── AdminService: GetStats ──────────────────────────────────────────────────

func TestAdminService_GetStats_NilPriceSvcNilAlertSvc(t *testing.T) {
	pg := &mockProviderStatusGetter{}

	subSvc := makeMinimalSubSvc()
	adminSvc := app.NewAdminService(pg, subSvc, nil, nil)

	stats := adminSvc.GetStats()
	assert.Equal(t, int64(0), stats.TotalRequests)
	assert.Equal(t, int64(0), stats.CacheHits)
	assert.Equal(t, float64(0), stats.CacheHitRate)
	assert.Equal(t, 2, stats.TotalProviders)
	assert.Equal(t, 2, stats.HealthyProviders)
	assert.Equal(t, 0, stats.ActiveAlerts)
	assert.Equal(t, 0, stats.TriggeredAlerts)
}

func TestAdminService_GetStats_WithPriceAndAlerts(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)
	provider := mocks.NewMockPriceProvider(ctrl)

	price := newTestPrice()
	// Two requests: one cache miss, one cache hit
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil)
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(price, nil)
	provider.EXPECT().Name().Return("p")
	router.EXPECT().ReportSuccess("p", gomock.Any())
	cache.EXPECT().Set(gomock.Any(), price, gomock.Any()).Return(nil)
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(price, nil)

	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router:   router,
		Cache:    cache,
		CacheTTL: 30 * time.Second,
	})

	_, _ = priceSvc.GetPrice(context.Background(), "bitcoin", "usd")
	_, _ = priceSvc.GetPrice(context.Background(), "bitcoin", "usd")

	alertSvc := app.NewAlertService()
	now := time.Now()
	triggeredAt := now.Add(-1 * time.Minute)

	// 2 active alerts
	_ = alertSvc.Create(&model.Alert{ID: "a1", CoinID: "bitcoin", Currency: "usd", Condition: model.ConditionAbove, Threshold: decimal.NewFromFloat(100000), WebhookURL: "https://hooks.example.com/webhook"})
	_ = alertSvc.Create(&model.Alert{ID: "a2", CoinID: "ethereum", Currency: "usd", Condition: model.ConditionBelow, Threshold: decimal.NewFromFloat(100), WebhookURL: "https://hooks.example.com/webhook"})
	// 1 triggered alert (injected via List then manually reading state — we trigger it via OnPriceUpdate)
	_ = alertSvc.Create(&model.Alert{ID: "a3", CoinID: "litecoin", Currency: "usd", Condition: model.ConditionAbove, Threshold: decimal.NewFromFloat(50), WebhookURL: "https://hooks.example.com/webhook"})
	// Manually set TriggeredAt to simulate triggered state
	alerts := alertSvc.List()
	for _, a := range alerts {
		if a.ID == "a3" {
			a.Active = false
			a.TriggeredAt = &triggeredAt
		}
	}

	subSvc := makeMinimalSubSvc()
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{}, subSvc, priceSvc, alertSvc)

	stats := adminSvc.GetStats()
	assert.Equal(t, int64(2), stats.TotalRequests)
	assert.Equal(t, int64(1), stats.CacheHits)
	assert.InDelta(t, 0.5, stats.CacheHitRate, 0.001)
	assert.GreaterOrEqual(t, stats.UptimeSeconds, int64(0))
}

func TestAdminService_GetStats_CacheHitRate_Zero(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	// No requests at all → rate = 0
	priceSvc := app.NewPriceService(app.PriceServiceDeps{Router: router})
	subSvc := makeMinimalSubSvc()
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{}, subSvc, priceSvc, nil)

	stats := adminSvc.GetStats()
	assert.Equal(t, float64(0), stats.CacheHitRate)
	assert.Equal(t, int64(0), stats.TotalRequests)
}

func TestAdminService_GetStats_NilSubSvc(t *testing.T) {
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{}, nil, nil, nil)
	stats := adminSvc.GetStats()
	assert.Equal(t, 0, stats.ActiveSubscriptions)
}

func TestAdminService_GetStats_AlertCounting(t *testing.T) {
	alertSvc := app.NewAlertService()

	// Create active alert
	_ = alertSvc.Create(&model.Alert{ID: "active1", CoinID: "btc", Currency: "usd", Condition: model.ConditionAbove, Threshold: decimal.NewFromFloat(100000), WebhookURL: "https://hooks.example.com/webhook"})

	subSvc := makeMinimalSubSvc()
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{}, subSvc, nil, alertSvc)

	stats := adminSvc.GetStats()
	assert.Equal(t, 1, stats.ActiveAlerts)
	assert.Equal(t, 0, stats.TriggeredAlerts)
}

func TestAdminService_GetStats_HealthyProviderCount(t *testing.T) {
	pg := &mixedHealthProviderGetter{}
	subSvc := makeMinimalSubSvc()
	adminSvc := app.NewAdminService(pg, subSvc, nil, nil)

	stats := adminSvc.GetStats()
	assert.Equal(t, 3, stats.TotalProviders)
	assert.Equal(t, 1, stats.HealthyProviders)
}

// ─── AdminService: subscription delegation ──────────────────────────────────

func TestAdminService_AddRemoveSubscription(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)

	price := &model.Price{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(67000), Provider: "mock", Timestamp: time.Now(), ReceivedAt: time.Now()}
	cache.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(price, nil).AnyTimes()

	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router: router, Cache: cache, CacheTTL: 30 * time.Second,
	})
	subSvc := app.NewSubscriptionService(context.Background(), priceSvc, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	subSvc.Start(ctx)
	defer subSvc.Stop()

	adminSvc := app.NewAdminService(&mockProviderStatusGetter{}, subSvc, priceSvc, nil)

	err := adminSvc.AddSubscription(ctx, "bitcoin", "usd", 1*time.Second)
	require.NoError(t, err)
	assert.Len(t, adminSvc.GetSubscriptions(), 1)

	err = adminSvc.RemoveSubscription("bitcoin", "usd")
	require.NoError(t, err)
	assert.Len(t, adminSvc.GetSubscriptions(), 0)
}

// ─── AnalyticsService ───────────────────────────────────────────────────────

func TestAnalyticsService_GetProviderAccuracy(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockAnalyticsStore(ctrl)

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	expected := []model.ProviderAccuracyReport{
		{Provider: "coingecko", Total: 100, Accurate: 95, AccuracyRate: 0.95},
		{Provider: "coinmarketcap", Total: 80, Accurate: 78, AccuracyRate: 0.975},
	}
	store.EXPECT().ProviderAccuracy(gomock.Any(), from, to).Return(expected, nil)

	svc := app.NewAnalyticsService(store)

	reports, err := svc.GetProviderAccuracy(context.Background(), from, to)
	require.NoError(t, err)
	assert.Len(t, reports, 2)
	assert.Equal(t, "coingecko", reports[0].Provider)
}

func TestAnalyticsService_GetProviderAccuracy_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockAnalyticsStore(ctrl)

	from := time.Now().Add(-time.Hour)
	to := time.Now()

	store.EXPECT().ProviderAccuracy(gomock.Any(), from, to).Return(nil, errors.New("db error"))

	svc := app.NewAnalyticsService(store)

	_, err := svc.GetProviderAccuracy(context.Background(), from, to)
	require.Error(t, err)
}

func TestAnalyticsService_GetAnomalies(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockAnalyticsStore(ctrl)

	from := time.Now().Add(-time.Hour)
	to := time.Now()
	deviation := 10.0

	expected := []model.PriceAnomaly{
		{CoinID: "bitcoin", Currency: "usd", Provider: "coingecko", Value: decimal.NewFromFloat(80000), MedianValue: decimal.NewFromFloat(67000), DeviationPct: 19.4},
	}
	store.EXPECT().PriceAnomalies(gomock.Any(), "bitcoin", "usd", deviation, from, to).Return(expected, nil)

	svc := app.NewAnalyticsService(store)

	anomalies, err := svc.GetAnomalies(context.Background(), "bitcoin", "usd", deviation, from, to)
	require.NoError(t, err)
	assert.Len(t, anomalies, 1)
	assert.Equal(t, "coingecko", anomalies[0].Provider)
}

func TestAnalyticsService_GetAnomalies_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockAnalyticsStore(ctrl)

	from := time.Now().Add(-time.Hour)
	to := time.Now()

	store.EXPECT().PriceAnomalies(gomock.Any(), "bitcoin", "usd", 5.0, from, to).Return(nil, errors.New("timeout"))

	svc := app.NewAnalyticsService(store)

	_, err := svc.GetAnomalies(context.Background(), "bitcoin", "usd", 5.0, from, to)
	require.Error(t, err)
}

func TestAnalyticsService_GetVolumeByProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockAnalyticsStore(ctrl)

	from := time.Now().Add(-time.Hour)
	to := time.Now()

	expected := []model.ProviderVolume{
		{Provider: "coingecko", QueryCount: 500, Percentage: 62.5},
		{Provider: "coinmarketcap", QueryCount: 300, Percentage: 37.5},
	}
	store.EXPECT().VolumeByProvider(gomock.Any(), from, to).Return(expected, nil)

	svc := app.NewAnalyticsService(store)

	volumes, err := svc.GetVolumeByProvider(context.Background(), from, to)
	require.NoError(t, err)
	assert.Len(t, volumes, 2)
	assert.Equal(t, "coingecko", volumes[0].Provider)
}

func TestAnalyticsService_GetVolumeByProvider_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockAnalyticsStore(ctrl)

	from := time.Now().Add(-time.Hour)
	to := time.Now()

	store.EXPECT().VolumeByProvider(gomock.Any(), from, to).Return(nil, errors.New("store error"))

	svc := app.NewAnalyticsService(store)

	_, err := svc.GetVolumeByProvider(context.Background(), from, to)
	require.Error(t, err)
}

func TestAnalyticsService_GetPriceStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockAnalyticsStore(ctrl)

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	expected := &model.PriceStatsSummary{
		CoinID:   "bitcoin",
		Currency: "usd",
		Min:      decimal.NewFromFloat(60000),
		Max:      decimal.NewFromFloat(70000),
		Avg:      decimal.NewFromFloat(65000),
		Stddev:   2500.0,
		Points:   1440,
	}
	store.EXPECT().PriceStats(gomock.Any(), "bitcoin", "usd", from, to).Return(expected, nil)

	svc := app.NewAnalyticsService(store)

	summary, err := svc.GetPriceStats(context.Background(), "bitcoin", "usd", from, to)
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Equal(t, "bitcoin", summary.CoinID)
	assert.True(t, decimal.NewFromFloat(60000).Equal(summary.Min))
}

func TestAnalyticsService_GetPriceStats_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockAnalyticsStore(ctrl)

	from := time.Now().Add(-time.Hour)
	to := time.Now()

	store.EXPECT().PriceStats(gomock.Any(), "bitcoin", "usd", from, to).Return(nil, errors.New("not found"))

	svc := app.NewAnalyticsService(store)

	_, err := svc.GetPriceStats(context.Background(), "bitcoin", "usd", from, to)
	require.Error(t, err)
}

// ─── SubscriptionService: edge cases ────────────────────────────────────────

func TestSubscriptionService_Subscriptions_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)

	priceSvc := app.NewPriceService(app.PriceServiceDeps{Router: router})
	subSvc := app.NewSubscriptionService(context.Background(), priceSvc, 10*time.Millisecond)

	subs := subSvc.Subscriptions()
	assert.Empty(t, subs)
}

func TestSubscriptionService_MultipleSubscriptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := mocks.NewMockProviderRouter(ctrl)
	cache := mocks.NewMockCache(ctrl)

	cache.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	router.EXPECT().Select(gomock.Any(), gomock.Any()).Return(nil, model.ErrNoHealthyProvider).AnyTimes()

	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router: router, Cache: cache, CacheTTL: 30 * time.Second,
	})
	subSvc := app.NewSubscriptionService(context.Background(), priceSvc, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	subSvc.Start(ctx)
	defer subSvc.Stop()

	err := subSvc.Subscribe(ctx, "bitcoin", "usd", 1*time.Second)
	require.NoError(t, err)
	err = subSvc.Subscribe(ctx, "ethereum", "usd", 2*time.Second)
	require.NoError(t, err)

	subs := subSvc.Subscriptions()
	assert.Len(t, subs, 2)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// makeMinimalSubSvc builds a SubscriptionService that uses a no-op price service
// with a router that always fails (safe for stats-only tests).
func makeMinimalSubSvc() *app.SubscriptionService {
	ctrl := gomock.NewController(nil) // nil T: we never assert calls here
	router := mocks.NewMockProviderRouter(ctrl)
	priceSvc := app.NewPriceService(app.PriceServiceDeps{Router: router})
	return app.NewSubscriptionService(context.Background(), priceSvc, 10*time.Millisecond)
}

// mixedHealthProviderGetter returns 1 healthy and 2 unhealthy providers.
type mixedHealthProviderGetter struct{}

func (m *mixedHealthProviderGetter) ProviderStatuses() []model.ProviderStatus {
	return []model.ProviderStatus{
		{Name: "coingecko", Healthy: true},
		{Name: "bad1", Healthy: false},
		{Name: "bad2", Healthy: false},
	}
}
