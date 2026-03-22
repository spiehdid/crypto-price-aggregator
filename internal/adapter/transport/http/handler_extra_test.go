package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	handler "github.com/spiehdid/crypto-price-aggregator/internal/adapter/transport/http"
	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// errCode extracts the "code" field from a nested {"error":{"code":"..."}} response body.
func errCode(t *testing.T, body []byte) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	return envelope.Error.Code
}

// newCachedPrice returns a minimal valid Price struct.
func newCachedPrice(coinID, currency string, value float64) *model.Price {
	return &model.Price{
		CoinID:     coinID,
		Currency:   currency,
		Value:      decimal.NewFromFloat(value),
		Provider:   "coingecko",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
	}
}

// setupPriceSvc creates a PriceService backed by mock cache + router.
func setupPriceSvc(t *testing.T) (*app.PriceService, *mocks.MockCache, *mocks.MockProviderRouter) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		Cache:      cache,
		CacheTTL:   30 * time.Second,
		MaxRetries: 1,
	})
	return svc, cache, router
}

// setupPriceSvcWithStore additionally accepts a PriceStore (needed for OHLC).
func setupPriceSvcWithStore(t *testing.T) (*app.PriceService, *mocks.MockCache, *mocks.MockProviderRouter, *mocks.MockPriceStore) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	store := mocks.NewMockPriceStore(ctrl)
	svc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		Cache:      cache,
		Store:      store,
		CacheTTL:   30 * time.Second,
		MaxRetries: 1,
	})
	return svc, cache, router, store
}

// newAnalyticsSvc builds an AnalyticsService backed by a mock store.
func newAnalyticsSvc(t *testing.T) (*app.AnalyticsService, *mocks.MockAnalyticsStore) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockAnalyticsStore(ctrl)
	svc := app.NewAnalyticsService(store)
	return svc, store
}

// ---------------------------------------------------------------------------
// HandleGetPrices
// ---------------------------------------------------------------------------

func TestHandleGetPrices_MissingParam(t *testing.T) {
	priceSvc, _, _ := setupPriceSvc(t)
	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/prices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "MISSING_PARAM", errCode(t, w.Body.Bytes()))
}

func TestHandleGetPrices_Success(t *testing.T) {
	priceSvc, cache, _ := setupPriceSvc(t)

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(newCachedPrice("bitcoin", "usd", 60000), nil)
	cache.EXPECT().Get(gomock.Any(), "ethereum", "usd").Return(newCachedPrice("ethereum", "usd", 3000), nil)

	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/prices?coins=bitcoin,ethereum", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotNil(t, resp["prices"])
	assert.NotNil(t, resp["errors"])
}

func TestHandleGetPrices_PartialError(t *testing.T) {
	priceSvc, cache, router := setupPriceSvc(t)

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(newCachedPrice("bitcoin", "usd", 60000), nil)
	cache.EXPECT().Get(gomock.Any(), "unknown-coin", "usd").Return(nil, nil)
	router.EXPECT().Select(gomock.Any(), "unknown-coin").Return(nil, model.ErrCoinNotFound)

	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/prices?coins=bitcoin,unknown-coin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errs, ok := resp["errors"].(map[string]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, errs["unknown-coin"])
}

func TestHandleGetPrices_DefaultCurrency(t *testing.T) {
	priceSvc, cache, _ := setupPriceSvc(t)
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "eur").Return(newCachedPrice("bitcoin", "eur", 55000), nil)

	h := handler.NewHandler(priceSvc, nil, nil, nil, "eur")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/prices?coins=bitcoin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ---------------------------------------------------------------------------
// HandleGetOHLC
// ---------------------------------------------------------------------------

func TestHandleGetOHLC_MissingFromTo(t *testing.T) {
	priceSvc, _, _ := setupPriceSvc(t)
	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/ohlc/bitcoin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "MISSING_PARAM", errCode(t, w.Body.Bytes()))
}

func TestHandleGetOHLC_InvalidFrom(t *testing.T) {
	priceSvc, _, _ := setupPriceSvc(t)
	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/ohlc/bitcoin?from=not-a-date&to=2024-01-02T00:00:00Z", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "INVALID_PARAM", errCode(t, w.Body.Bytes()))
}

func TestHandleGetOHLC_InvalidTo(t *testing.T) {
	priceSvc, _, _ := setupPriceSvc(t)
	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/ohlc/bitcoin?from=2024-01-01T00:00:00Z&to=not-a-date", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "INVALID_PARAM", errCode(t, w.Body.Bytes()))
}

func TestHandleGetOHLC_Success(t *testing.T) {
	priceSvc, _, _, store := setupPriceSvcWithStore(t)

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	candles := []model.OHLC{
		{
			Open:  decimal.NewFromFloat(60000),
			High:  decimal.NewFromFloat(62000),
			Low:   decimal.NewFromFloat(59000),
			Close: decimal.NewFromFloat(61000),
			Time:  from,
		},
	}
	store.EXPECT().GetOHLC(gomock.Any(), "bitcoin", "usd", from, to, time.Hour).Return(candles, nil)

	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	url := fmt.Sprintf("/api/v1/ohlc/bitcoin?from=%s&to=%s&interval=1h",
		from.Format(time.RFC3339), to.Format(time.RFC3339))
	req := httptest.NewRequest("GET", url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "bitcoin", resp["coin"])
	assert.Equal(t, "usd", resp["currency"])
}

func TestHandleGetOHLC_StoreError(t *testing.T) {
	priceSvc, _, _, store := setupPriceSvcWithStore(t)

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	store.EXPECT().GetOHLC(gomock.Any(), "bitcoin", "usd", from, to, time.Hour).Return(nil, errors.New("db error"))

	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	url := fmt.Sprintf("/api/v1/ohlc/bitcoin?from=%s&to=%s&interval=1h",
		from.Format(time.RFC3339), to.Format(time.RFC3339))
	req := httptest.NewRequest("GET", url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "INTERNAL_ERROR", errCode(t, w.Body.Bytes()))
}

// ---------------------------------------------------------------------------
// HandleConvert
// ---------------------------------------------------------------------------

func TestHandleConvert_MissingParams(t *testing.T) {
	priceSvc, _, _ := setupPriceSvc(t)
	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/convert?from=bitcoin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "MISSING_PARAM", errCode(t, w.Body.Bytes()))
}

func TestHandleConvert_InvalidAmount(t *testing.T) {
	priceSvc, _, _ := setupPriceSvc(t)
	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/convert?from=bitcoin&to=ethereum&amount=not-a-number", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "INVALID_PARAM", errCode(t, w.Body.Bytes()))
}

func TestHandleConvert_Success(t *testing.T) {
	priceSvc, cache, _ := setupPriceSvc(t)

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(newCachedPrice("bitcoin", "usd", 60000), nil)
	cache.EXPECT().Get(gomock.Any(), "ethereum", "usd").Return(newCachedPrice("ethereum", "usd", 3000), nil)

	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/convert?from=bitcoin&to=ethereum&amount=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "bitcoin", resp["from"])
	assert.Equal(t, "ethereum", resp["to"])
	assert.Equal(t, "1", resp["amount"])
}

func TestHandleConvert_DefaultAmount(t *testing.T) {
	priceSvc, cache, _ := setupPriceSvc(t)

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(newCachedPrice("bitcoin", "usd", 60000), nil)
	cache.EXPECT().Get(gomock.Any(), "ethereum", "usd").Return(newCachedPrice("ethereum", "usd", 3000), nil)

	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	// No amount param — should default to 1
	req := httptest.NewRequest("GET", "/api/v1/convert?from=bitcoin&to=ethereum", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleConvert_PriceError(t *testing.T) {
	priceSvc, cache, router := setupPriceSvc(t)

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(nil, model.ErrNoHealthyProvider)

	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/convert?from=bitcoin&to=ethereum&amount=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// ---------------------------------------------------------------------------
// HandleCreateAlert
// ---------------------------------------------------------------------------

func TestHandleCreateAlert_InvalidBody(t *testing.T) {
	h := handler.NewHandler(nil, nil, app.NewAlertService(), nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("POST", "/api/v1/alerts", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "INVALID_BODY", errCode(t, w.Body.Bytes()))
}

func TestHandleCreateAlert_MissingFields(t *testing.T) {
	h := handler.NewHandler(nil, nil, app.NewAlertService(), nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	body, _ := json.Marshal(map[string]string{"coin_id": "bitcoin"})
	req := httptest.NewRequest("POST", "/api/v1/alerts", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "MISSING_PARAM", errCode(t, w.Body.Bytes()))
}

func TestHandleCreateAlert_InvalidThreshold(t *testing.T) {
	h := handler.NewHandler(nil, nil, app.NewAlertService(), nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	body, _ := json.Marshal(map[string]string{
		"coin_id":     "bitcoin",
		"threshold":   "not-a-number",
		"webhook_url": "https://example.com/hook",
	})
	req := httptest.NewRequest("POST", "/api/v1/alerts", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "INVALID_PARAM", errCode(t, w.Body.Bytes()))
}

func TestHandleCreateAlert_Success_Above(t *testing.T) {
	h := handler.NewHandler(nil, nil, app.NewAlertService(), nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	body, _ := json.Marshal(map[string]string{
		"coin_id":     "bitcoin",
		"condition":   "above",
		"threshold":   "70000",
		"webhook_url": "https://example.com/hook",
	})
	req := httptest.NewRequest("POST", "/api/v1/alerts", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["ID"])
}

func TestHandleCreateAlert_Success_Below(t *testing.T) {
	h := handler.NewHandler(nil, nil, app.NewAlertService(), nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	body, _ := json.Marshal(map[string]string{
		"coin_id":     "ethereum",
		"condition":   "below",
		"threshold":   "1000",
		"webhook_url": "https://example.com/hook",
		"currency":    "eur",
	})
	req := httptest.NewRequest("POST", "/api/v1/alerts", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

// ---------------------------------------------------------------------------
// HandleListAlerts
// ---------------------------------------------------------------------------

func TestHandleListAlerts_Empty(t *testing.T) {
	h := handler.NewHandler(nil, nil, app.NewAlertService(), nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/alerts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	alerts, ok := resp["alerts"].([]interface{})
	require.True(t, ok)
	assert.Empty(t, alerts)
}

func TestHandleListAlerts_WithAlerts(t *testing.T) {
	alertSvc := app.NewAlertService()
	_ = alertSvc.Create(&model.Alert{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Condition:  model.ConditionAbove,
		Threshold:  decimal.NewFromFloat(70000),
		WebhookURL: "https://example.com/hook",
	})

	h := handler.NewHandler(nil, nil, alertSvc, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/alerts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	alerts, ok := resp["alerts"].([]interface{})
	require.True(t, ok)
	assert.Len(t, alerts, 1)
}

// ---------------------------------------------------------------------------
// HandleDeleteAlert
// ---------------------------------------------------------------------------

func TestHandleDeleteAlert_NotFound(t *testing.T) {
	h := handler.NewHandler(nil, nil, app.NewAlertService(), nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("DELETE", "/api/v1/alerts/nonexistent-id", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ALERT_NOT_FOUND", errCode(t, w.Body.Bytes()))
}

func TestHandleDeleteAlert_Success(t *testing.T) {
	alertSvc := app.NewAlertService()
	alert := &model.Alert{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Condition:  model.ConditionAbove,
		Threshold:  decimal.NewFromFloat(70000),
		WebhookURL: "https://example.com/hook",
	}
	require.NoError(t, alertSvc.Create(alert))

	h := handler.NewHandler(nil, nil, alertSvc, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("DELETE", "/api/v1/alerts/"+alert.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "deleted", resp["status"])
}

// ---------------------------------------------------------------------------
// HandleGetSubscriptions
// ---------------------------------------------------------------------------

func TestHandleGetSubscriptions(t *testing.T) {
	// newAdminSvc has nil subSvc which would panic on GetSubscriptions.
	// Build one with a real SubscriptionService instead.
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router:     router,
		Cache:      cache,
		CacheTTL:   30 * time.Second,
		MaxRetries: 1,
	})
	subSvc := app.NewSubscriptionService(context.Background(), priceSvc, time.Second)
	adminSvcWithSub := app.NewAdminService(&mockProviderStatusGetter{healthy: true}, subSvc, nil, nil)

	h := handler.NewHandler(nil, adminSvcWithSub, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/admin/subscriptions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotNil(t, resp["subscriptions"])
}

// ---------------------------------------------------------------------------
// HandlePostSubscription
// ---------------------------------------------------------------------------

func TestHandlePostSubscription_InvalidBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router: router, Cache: cache, CacheTTL: 30 * time.Second, MaxRetries: 1,
	})
	subSvc := app.NewSubscriptionService(context.Background(), priceSvc, time.Second)
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{healthy: true}, subSvc, nil, nil)

	h := handler.NewHandler(nil, adminSvc, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("POST", "/api/v1/admin/subscriptions", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "INVALID_BODY", errCode(t, w.Body.Bytes()))
}

func TestHandlePostSubscription_MissingFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router: router, Cache: cache, CacheTTL: 30 * time.Second, MaxRetries: 1,
	})
	subSvc := app.NewSubscriptionService(context.Background(), priceSvc, time.Second)
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{healthy: true}, subSvc, nil, nil)

	h := handler.NewHandler(nil, adminSvc, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	body, _ := json.Marshal(map[string]string{"coin_id": "bitcoin"})
	req := httptest.NewRequest("POST", "/api/v1/admin/subscriptions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "MISSING_PARAM", errCode(t, w.Body.Bytes()))
}

// newSubAdminSvc builds a SubscriptionService + AdminService with a started poller context.
// The background goroutine may call cache.Get/router.Select; callers must add AnyTimes expectations.
func newSubAdminSvc(
	priceSvc *app.PriceService,
	ctx context.Context,
) (*app.AdminService, *app.SubscriptionService) {
	subSvc := app.NewSubscriptionService(context.Background(), priceSvc, time.Second)
	subSvc.Start(ctx)
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{healthy: true}, subSvc, nil, nil)
	return adminSvc, subSvc
}

func TestHandlePostSubscription_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router: router, Cache: cache, CacheTTL: 30 * time.Second, MaxRetries: 1,
	})

	// Allow any background poll calls from the poller goroutine.
	cache.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	router.EXPECT().Select(gomock.Any(), gomock.Any()).Return(nil, model.ErrNoHealthyProvider).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	adminSvc, _ := newSubAdminSvc(priceSvc, ctx)

	h := handler.NewHandler(nil, adminSvc, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	body, _ := json.Marshal(map[string]string{
		"coin_id":  "bitcoin",
		"currency": "usd",
		"interval": "30s",
	})
	req := httptest.NewRequest("POST", "/api/v1/admin/subscriptions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "subscribed", resp["status"])
}

func TestHandlePostSubscription_DefaultInterval(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router: router, Cache: cache, CacheTTL: 30 * time.Second, MaxRetries: 1,
	})

	// Allow any background poll calls from the poller goroutine.
	cache.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	router.EXPECT().Select(gomock.Any(), gomock.Any()).Return(nil, model.ErrNoHealthyProvider).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	adminSvc, _ := newSubAdminSvc(priceSvc, ctx)

	h := handler.NewHandler(nil, adminSvc, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	// interval is invalid — should default to 60s
	body, _ := json.Marshal(map[string]string{
		"coin_id":  "ethereum",
		"currency": "usd",
		"interval": "bad-interval",
	})
	req := httptest.NewRequest("POST", "/api/v1/admin/subscriptions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

// ---------------------------------------------------------------------------
// HandleDeleteSubscription
// ---------------------------------------------------------------------------

func TestHandleDeleteSubscription_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router: router, Cache: cache, CacheTTL: 30 * time.Second, MaxRetries: 1,
	})
	subSvc := app.NewSubscriptionService(context.Background(), priceSvc, time.Second)
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{healthy: true}, subSvc, nil, nil)

	h := handler.NewHandler(nil, adminSvc, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/subscriptions/nonexistent?currency=usd", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "SUBSCRIPTION_NOT_FOUND", errCode(t, w.Body.Bytes()))
}

func TestHandleDeleteSubscription_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router: router, Cache: cache, CacheTTL: 30 * time.Second, MaxRetries: 1,
	})

	// Allow background poll goroutine to call cache/router without strict expectations.
	cache.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	router.EXPECT().Select(gomock.Any(), gomock.Any()).Return(nil, model.ErrNoHealthyProvider).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	subSvc := app.NewSubscriptionService(context.Background(), priceSvc, time.Second)
	subSvc.Start(ctx)
	require.NoError(t, subSvc.Subscribe(ctx, "bitcoin", "usd", 30*time.Second))

	adminSvc := app.NewAdminService(&mockProviderStatusGetter{healthy: true}, subSvc, nil, nil)

	h := handler.NewHandler(nil, adminSvc, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/subscriptions/bitcoin?currency=usd", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "unsubscribed", resp["status"])
}

// ---------------------------------------------------------------------------
// HandleGetStats
// ---------------------------------------------------------------------------

func TestHandleGetStats(t *testing.T) {
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{healthy: true}, nil, nil, nil)
	h := handler.NewHandler(nil, adminSvc, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "total_requests")
	assert.Contains(t, resp, "healthy_providers")
}

// ---------------------------------------------------------------------------
// HandleGetPriceByAddress
// ---------------------------------------------------------------------------

func TestHandleGetPriceByAddress_NotFound(t *testing.T) {
	priceSvc, _, _ := setupPriceSvc(t)
	// No registry, no token providers -> ErrCoinNotFound
	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/price/address/ethereum/0xdeadbeef", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "COIN_NOT_FOUND", errCode(t, w.Body.Bytes()))
}

func TestHandleGetPriceByAddress_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	cache := mocks.NewMockCache(ctrl)
	router := mocks.NewMockProviderRouter(ctrl)
	tokenProvider := mocks.NewMockTokenPriceProvider(ctrl)

	tokenProvider.EXPECT().SupportsChain("ethereum").Return(true)
	tokenProvider.EXPECT().GetPriceByAddress(gomock.Any(), "ethereum", "0xdeadbeef", "usd").Return(
		newCachedPrice("sometoken", "usd", 1.5), nil,
	)

	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router:         router,
		Cache:          cache,
		CacheTTL:       30 * time.Second,
		MaxRetries:     1,
		TokenProviders: []port.TokenPriceProvider{tokenProvider},
	})

	h := handler.NewHandler(priceSvc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/price/address/ethereum/0xdeadbeef", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ethereum", resp["chain"])
	assert.Equal(t, "0xdeadbeef", resp["address"])
}

// ---------------------------------------------------------------------------
// Analytics handlers — nil analyticsSvc => 503
// ---------------------------------------------------------------------------

func TestHandleProviderAccuracy_NilAnalytics(t *testing.T) {
	h := handler.NewHandler(nil, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/provider-accuracy", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ANALYTICS_DISABLED", errCode(t, w.Body.Bytes()))
}

func TestHandleProviderAccuracy_Success(t *testing.T) {
	analyticsSvc, store := newAnalyticsSvc(t)
	store.EXPECT().ProviderAccuracy(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		[]model.ProviderAccuracyReport{{Provider: "coingecko", Total: 100, Accurate: 95, AccuracyRate: 0.95}},
		nil,
	)

	h := handler.NewHandler(nil, nil, nil, analyticsSvc, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/provider-accuracy", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	providers, ok := resp["providers"].([]interface{})
	require.True(t, ok)
	assert.Len(t, providers, 1)
}

func TestHandleProviderAccuracy_Error(t *testing.T) {
	analyticsSvc, store := newAnalyticsSvc(t)
	store.EXPECT().ProviderAccuracy(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))

	h := handler.NewHandler(nil, nil, nil, analyticsSvc, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/provider-accuracy", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "INTERNAL_ERROR", errCode(t, w.Body.Bytes()))
}

// ---------------------------------------------------------------------------
// HandlePriceAnomalies
// ---------------------------------------------------------------------------

func TestHandlePriceAnomalies_NilAnalytics(t *testing.T) {
	h := handler.NewHandler(nil, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/anomalies", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandlePriceAnomalies_Success(t *testing.T) {
	analyticsSvc, store := newAnalyticsSvc(t)
	store.EXPECT().PriceAnomalies(gomock.Any(), "bitcoin", "usd", 5.0, gomock.Any(), gomock.Any()).Return(
		[]model.PriceAnomaly{{Provider: "coingecko", CoinID: "bitcoin", Currency: "usd"}},
		nil,
	)

	h := handler.NewHandler(nil, nil, nil, analyticsSvc, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/anomalies?coin=bitcoin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	anomalies, ok := resp["anomalies"].([]interface{})
	require.True(t, ok)
	assert.Len(t, anomalies, 1)
}

func TestHandlePriceAnomalies_CustomDeviation(t *testing.T) {
	analyticsSvc, store := newAnalyticsSvc(t)
	store.EXPECT().PriceAnomalies(gomock.Any(), "bitcoin", "usd", 10.0, gomock.Any(), gomock.Any()).Return(
		[]model.PriceAnomaly{},
		nil,
	)

	h := handler.NewHandler(nil, nil, nil, analyticsSvc, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/anomalies?coin=bitcoin&deviation=10.0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandlePriceAnomalies_Error(t *testing.T) {
	analyticsSvc, store := newAnalyticsSvc(t)
	store.EXPECT().PriceAnomalies(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("db error"))

	h := handler.NewHandler(nil, nil, nil, analyticsSvc, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/anomalies", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------------------------------------------------------------------------
// HandleVolumeByProvider
// ---------------------------------------------------------------------------

func TestHandleVolumeByProvider_NilAnalytics(t *testing.T) {
	h := handler.NewHandler(nil, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/volume-by-provider", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleVolumeByProvider_Success(t *testing.T) {
	analyticsSvc, store := newAnalyticsSvc(t)
	store.EXPECT().VolumeByProvider(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		[]model.ProviderVolume{{Provider: "coingecko", QueryCount: 500, Percentage: 100.0}},
		nil,
	)

	h := handler.NewHandler(nil, nil, nil, analyticsSvc, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/volume-by-provider", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	providers, ok := resp["providers"].([]interface{})
	require.True(t, ok)
	assert.Len(t, providers, 1)
}

func TestHandleVolumeByProvider_Error(t *testing.T) {
	analyticsSvc, store := newAnalyticsSvc(t)
	store.EXPECT().VolumeByProvider(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))

	h := handler.NewHandler(nil, nil, nil, analyticsSvc, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/volume-by-provider", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------------------------------------------------------------------------
// HandlePriceStats
// ---------------------------------------------------------------------------

func TestHandlePriceStats_NilAnalytics(t *testing.T) {
	h := handler.NewHandler(nil, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/price-stats?coin=bitcoin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandlePriceStats_MissingCoin(t *testing.T) {
	analyticsSvc, _ := newAnalyticsSvc(t)
	h := handler.NewHandler(nil, nil, nil, analyticsSvc, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/price-stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "MISSING_PARAM", errCode(t, w.Body.Bytes()))
}

func TestHandlePriceStats_Success(t *testing.T) {
	analyticsSvc, store := newAnalyticsSvc(t)
	store.EXPECT().PriceStats(gomock.Any(), "bitcoin", "usd", gomock.Any(), gomock.Any()).Return(
		&model.PriceStatsSummary{
			CoinID:   "bitcoin",
			Currency: "usd",
			Min:      decimal.NewFromFloat(55000),
			Max:      decimal.NewFromFloat(70000),
			Avg:      decimal.NewFromFloat(62500),
			Stddev:   3000.0,
			Points:   100,
		},
		nil,
	)

	h := handler.NewHandler(nil, nil, nil, analyticsSvc, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/price-stats?coin=bitcoin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "bitcoin", resp["coin_id"])
}

func TestHandlePriceStats_Error(t *testing.T) {
	analyticsSvc, store := newAnalyticsSvc(t)
	store.EXPECT().PriceStats(gomock.Any(), "bitcoin", "usd", gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))

	h := handler.NewHandler(nil, nil, nil, analyticsSvc, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/price-stats?coin=bitcoin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandlePriceStats_CustomCurrency(t *testing.T) {
	analyticsSvc, store := newAnalyticsSvc(t)
	store.EXPECT().PriceStats(gomock.Any(), "bitcoin", "eur", gomock.Any(), gomock.Any()).Return(
		&model.PriceStatsSummary{CoinID: "bitcoin", Currency: "eur"},
		nil,
	)

	h := handler.NewHandler(nil, nil, nil, analyticsSvc, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/analytics/price-stats?coin=bitcoin&currency=eur", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ---------------------------------------------------------------------------
// HandleGetPrice — additional edge cases
// ---------------------------------------------------------------------------

func TestHandleGetPrice_WithMarketCapAndVolume(t *testing.T) {
	svc, cache, _, _ := setupService(t)

	cached := &model.Price{
		CoinID:    "bitcoin",
		Currency:  "usd",
		Value:     decimal.NewFromFloat(67000),
		Provider:  "coingecko",
		Timestamp: time.Now(),
		MarketCap: decimal.NewFromFloat(1_300_000_000_000),
		Volume24h: decimal.NewFromFloat(30_000_000_000),
		Change24h: 2.5,
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
	assert.NotNil(t, resp["market_cap"])
	assert.NotNil(t, resp["volume_24h"])
	assert.NotNil(t, resp["change_24h"])
}

func TestHandleGetPrice_ExplicitCurrency(t *testing.T) {
	svc, cache, _, _ := setupService(t)

	cached := &model.Price{
		CoinID:    "bitcoin",
		Currency:  "eur",
		Value:     decimal.NewFromFloat(62000),
		Provider:  "coingecko",
		Timestamp: time.Now(),
	}
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "eur").Return(cached, nil)

	h := handler.NewHandler(svc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/price/bitcoin?currency=eur", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "eur", resp["currency"])
}

func TestHandleGetPrice_BudgetExhausted(t *testing.T) {
	svc, cache, router, _ := setupService(t)
	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(nil, model.ErrBudgetExhausted)

	h := handler.NewHandler(svc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/price/bitcoin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "BUDGET_EXHAUSTED", errCode(t, w.Body.Bytes()))
}

func TestHandleGetPrice_InternalError(t *testing.T) {
	// Use setupPriceSvc (MaxRetries:1) so there is exactly one Select call.
	svc, cache, router := setupPriceSvc(t)
	ctrl := gomock.NewController(t)
	provider := mocks.NewMockPriceProvider(ctrl)

	cache.EXPECT().Get(gomock.Any(), "bitcoin", "usd").Return(nil, nil)
	router.EXPECT().Select(gomock.Any(), "bitcoin").Return(provider, nil)
	provider.EXPECT().GetPrice(gomock.Any(), "bitcoin", "usd").Return(nil, errors.New("unexpected failure"))
	provider.EXPECT().Name().Return("coingecko").AnyTimes()
	router.EXPECT().ReportFailure("coingecko", gomock.Any())

	h := handler.NewHandler(svc, nil, nil, nil, "usd")
	r := handler.NewRouter(h, handler.RouterConfig{})

	req := httptest.NewRequest("GET", "/api/v1/price/bitcoin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "INTERNAL_ERROR", errCode(t, w.Body.Bytes()))
}
