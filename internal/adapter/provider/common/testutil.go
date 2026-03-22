package common

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
	"github.com/stretchr/testify/assert"
)

// ProviderFactory creates a provider from a base URL for testing.
type ProviderFactory func(baseURL string) port.PriceProvider

// TestMalformedJSON tests that the provider handles malformed JSON gracefully.
func TestMalformedJSON(t *testing.T, name string, factory ProviderFactory) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	p := factory(server.URL)
	_, err := p.GetPrice(context.Background(), "bitcoin", "usd")
	assert.Error(t, err, "%s should error on malformed JSON", name)
}

// TestEmptyBody tests that the provider handles an empty 200 response.
func TestEmptyBody(t *testing.T, name string, factory ProviderFactory) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// empty body
	}))
	defer server.Close()

	p := factory(server.URL)
	_, err := p.GetPrice(context.Background(), "bitcoin", "usd")
	assert.Error(t, err, "%s should error on empty body", name)
}

// TestServerErrors tests various server error status codes.
func TestServerErrors(t *testing.T, name string, factory ProviderFactory) {
	t.Helper()
	codes := []int{403, 500, 502, 503}
	for _, code := range codes {
		code := code
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			p := factory(server.URL)
			_, err := p.GetPrice(context.Background(), "bitcoin", "usd")
			assert.Error(t, err, "%s should error on %d", name, code)
		})
	}
}

// TestTimeout tests that the provider respects context cancellation.
func TestTimeout(t *testing.T, name string, factory ProviderFactory) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // hang
	}))
	defer server.Close()

	p := factory(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := p.GetPrice(ctx, "bitcoin", "usd")
	assert.Error(t, err, "%s should error on timeout", name)
}
