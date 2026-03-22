package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	handler "github.com/spiehdid/crypto-price-aggregator/internal/adapter/transport/http"
	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type mockProviderStatusGetter struct{ healthy bool }

func (m *mockProviderStatusGetter) ProviderStatuses() []model.ProviderStatus {
	return []model.ProviderStatus{
		{Name: "coingecko", Healthy: m.healthy, Tier: model.TierFree, RemainingRate: 25, AvgLatency: 100 * time.Millisecond},
	}
}

func setupService(t *testing.T) (*app.PriceService, *mocks.MockCache, *mocks.MockProviderRouter, *mocks.MockPriceProvider) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	provider := mocks.NewMockPriceProvider(ctrl)

	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		Cache:      cache,
		CacheTTL:   30 * time.Second,
		MaxRetries: 3,
	})

	return svc, cache, router, provider
}

func TestHandleGetPrice_Success(t *testing.T) {
	svc, cache, _, _ := setupService(t)

	cached := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(67000), Provider: "coingecko",
		Timestamp: time.Now(), ReceivedAt: time.Now(),
	}
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(cached, nil)

	h := handler.NewHandler(svc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/price/bitcoin", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "bitcoin", resp["coin"])
	assert.Equal(t, "usd", resp["currency"])
}

func TestHandleGetPrice_NotFound(t *testing.T) {
	svc, cache, router, _ := setupService(t)

	cache.EXPECT().Get(gomock.Any(), "nonexistent", "usd").Return(nil, nil)
	router.EXPECT().Select(gomock.Any(), "nonexistent").Return(nil, model.ErrNoHealthyProvider)

	h := handler.NewHandler(svc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/price/nonexistent", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleHealthz(t *testing.T) {
	svc, _, _, _ := setupService(t)
	h := handler.NewHandler(svc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleGetProviders(t *testing.T) {
	svc, _, _, _ := setupService(t)
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{healthy: true}, nil, nil, nil)
	h := handler.NewHandler(svc, adminSvc, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/admin/providers", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	providers, ok := resp["providers"].([]interface{})
	require.True(t, ok)
	assert.Len(t, providers, 1)
	first, ok := providers[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "coingecko", first["Name"])
}

func TestHandleReadyz_Healthy(t *testing.T) {
	svc, _, _, _ := setupService(t)
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{healthy: true}, nil, nil, nil)
	h := handler.NewHandler(svc, adminSvc, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ready", resp["status"])
}

func TestHandleReadyz_Unhealthy(t *testing.T) {
	svc, _, _, _ := setupService(t)
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{healthy: false}, nil, nil, nil)
	h := handler.NewHandler(svc, adminSvc, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "not ready", resp["status"])
}
