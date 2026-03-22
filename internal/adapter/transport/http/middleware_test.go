package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	handler "github.com/spiehdid/crypto-price-aggregator/internal/adapter/transport/http"
	"github.com/stretchr/testify/assert"
)

func TestAuthMiddleware_NoKey_PassThrough(t *testing.T) {
	mw := handler.AuthMiddleware("")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_ValidKey(t *testing.T) {
	mw := handler.AuthMiddleware("secret-key")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "secret-key")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_InvalidKey(t *testing.T) {
	mw := handler.AuthMiddleware("secret-key")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_MissingKey(t *testing.T) {
	mw := handler.AuthMiddleware("secret-key")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRateLimitMiddleware_AllowsBurst(t *testing.T) {
	mw := handler.RateLimitMiddleware(10, 5)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
}

func TestRateLimitMiddleware_RejectsOverBurst(t *testing.T) {
	mw := handler.RateLimitMiddleware(1, 2)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	var lastCode int
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		lastCode = w.Code
	}
	assert.Equal(t, http.StatusTooManyRequests, lastCode)
}
