package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	handler "github.com/spiehdid/crypto-price-aggregator/internal/adapter/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// Test 7: Different IPs get separate rate limiters.
func TestRateLimitMiddleware_SeparatePerIP(t *testing.T) {
	// rps=1, burst=5: each IP can burst 5 requests.
	mw := handler.RateLimitMiddleware(1, 5)
	h := mw(okHandler)

	// IP 1.2.3.4 sends 5 requests — all should pass.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:1111"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "IP 1.2.3.4 request %d should pass", i+1)
	}

	// IP 5.6.7.8 sends 5 requests — all should also pass (separate limiter).
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "5.6.7.8:2222"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "IP 5.6.7.8 request %d should pass", i+1)
	}
}

// Test 8: Same IP over burst gets 429 with Retry-After header.
func TestRateLimitMiddleware_OverBurstReturns429WithRetryAfter(t *testing.T) {
	burst := 3
	mw := handler.RateLimitMiddleware(1, burst)
	h := mw(okHandler)

	// Send burst requests — all should pass.
	for i := 0; i < burst; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:3333"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "request %d within burst should pass", i+1)
	}

	// Next request exceeds burst: should get 429.
	// Send several more to ensure at least one is rejected (token bucket may have refilled a tiny bit).
	got429 := false
	var retryAfter string
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:3333"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			got429 = true
			retryAfter = w.Header().Get("Retry-After")
			break
		}
	}

	require.True(t, got429, "should have received 429 after exceeding burst")
	assert.NotEmpty(t, retryAfter, "Retry-After header must be present on 429 response")
}

// Test 9: CORS preflight OPTIONS request.
func TestCORSMiddleware_Preflight(t *testing.T) {
	mw := handler.CORSMiddleware([]string{"https://example.com"})
	h := mw(okHandler)

	req := httptest.NewRequest("OPTIONS", "/api/v1/prices", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "OPTIONS preflight should return 204")
	assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Max-Age"))
}

// Test 10: CORS blocked origin gets no CORS headers.
func TestCORSMiddleware_BlockedOrigin(t *testing.T) {
	mw := handler.CORSMiddleware([]string{"https://example.com"})
	h := mw(okHandler)

	req := httptest.NewRequest("GET", "/api/v1/prices", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"),
		"blocked origin should NOT get Access-Control-Allow-Origin header")
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Methods"),
		"blocked origin should NOT get Access-Control-Allow-Methods header")
}

// Test 11: Security headers are always present on every response.
func TestSecurityHeaders_Present(t *testing.T) {
	mw := handler.SecurityHeadersMiddleware(false)
	h := mw(okHandler)

	req := httptest.NewRequest("GET", "/anything", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "1; mode=block", w.Header().Get("X-XSS-Protection"))

	// HSTS should NOT be present when forceHTTPS=false.
	assert.Empty(t, w.Header().Get("Strict-Transport-Security"),
		"HSTS should not be set when forceHTTPS is false")

	// With forceHTTPS=true, HSTS should be present.
	mwHTTPS := handler.SecurityHeadersMiddleware(true)
	hHTTPS := mwHTTPS(okHandler)
	req2 := httptest.NewRequest("GET", "/anything", nil)
	w2 := httptest.NewRecorder()
	hHTTPS.ServeHTTP(w2, req2)
	assert.NotEmpty(t, w2.Header().Get("Strict-Transport-Security"),
		"HSTS should be set when forceHTTPS is true")
}

// Test 12: Auth middleware with empty key means auth is disabled.
func TestAuthMiddleware_EmptyKey_DisabledAuth(t *testing.T) {
	mw := handler.AuthMiddleware("")
	h := mw(okHandler)

	// Request WITHOUT any X-API-Key header should pass through.
	req := httptest.NewRequest("GET", "/admin/something", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"empty API key config should disable auth, allowing all requests through")

	// Even with a random key, should still pass.
	req2 := httptest.NewRequest("GET", "/admin/something", nil)
	req2.Header.Set("X-API-Key", "random-garbage")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code,
		"empty API key config should ignore any provided key")
}
