package registry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spiehdid/crypto-price-aggregator/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Add duplicate address overrides the coin ID ---

func TestRegistry_AddDuplicateOverrides(t *testing.T) {
	r := registry.New()
	r.Add("ethereum", "0xabc", "tether")
	r.Add("ethereum", "0xabc", "usdc") // same address, duplicate — first write wins

	coinID, found := r.Resolve("ethereum", "0xabc")
	assert.True(t, found)
	// First Add wins; duplicate is ignored.
	assert.Equal(t, "tether", coinID)
}

// --- LoadCatalog with empty platforms map (no entries added) ---

func TestRegistry_LoadCatalog_EmptyPlatforms(t *testing.T) {
	r := registry.New()
	catalog := []registry.CoinEntry{
		{ID: "bitcoin", Symbol: "btc", Platforms: map[string]string{}},
	}
	chains := map[string]bool{"ethereum": true}
	r.LoadCatalog(catalog, chains)

	assert.Equal(t, 0, r.Count())
}

// --- LoadCatalog skips entries with empty address strings ---

func TestRegistry_LoadCatalog_SkipsEmptyAddresses(t *testing.T) {
	r := registry.New()
	catalog := []registry.CoinEntry{
		{
			ID:     "bitcoin",
			Symbol: "btc",
			Platforms: map[string]string{
				"ethereum": "", // empty — must be skipped
				"tron":     "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
			},
		},
	}
	chains := map[string]bool{"ethereum": true, "tron": true}
	r.LoadCatalog(catalog, chains)

	// Only the tron entry should have been added.
	assert.Equal(t, 1, r.Count())
	_, found := r.Resolve("ethereum", "")
	assert.False(t, found)
}

// --- Count after LoadCatalog ---

func TestRegistry_CountAfterLoadCatalog(t *testing.T) {
	r := registry.New()
	catalog := []registry.CoinEntry{
		{
			ID:     "tether",
			Symbol: "usdt",
			Platforms: map[string]string{
				"ethereum": "0xdac17f958d2ee523a2206206994597c13d831ec7",
				"tron":     "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
			},
		},
		{
			ID:     "usd-coin",
			Symbol: "usdc",
			Platforms: map[string]string{
				"ethereum": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
			},
		},
	}
	chains := map[string]bool{"ethereum": true, "tron": true}
	r.LoadCatalog(catalog, chains)

	// 2 ethereum + 1 tron = 3 distinct address keys.
	assert.Equal(t, 3, r.Count())
}

// --- FetchCoinGeckoCatalog: success ---

func TestFetchCoinGeckoCatalog_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/coins/list")
		assert.Equal(t, "true", r.URL.Query().Get("include_platform"))
		_, _ = w.Write([]byte(`[{"id":"bitcoin","symbol":"btc","platforms":{"ethereum":"0x123"}},{"id":"tether","symbol":"usdt","platforms":{"ethereum":"0xdac"}}]`))
	}))
	defer server.Close()

	entries, err := registry.FetchCoinGeckoCatalog(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "bitcoin", entries[0].ID)
	assert.Equal(t, "tether", entries[1].ID)
}

// --- FetchCoinGeckoCatalog: non-200 status returns error ---

func TestFetchCoinGeckoCatalog_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := registry.FetchCoinGeckoCatalog(context.Background(), server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}

// --- FetchCoinGeckoCatalog: invalid JSON returns error ---

func TestFetchCoinGeckoCatalog_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not valid json`))
	}))
	defer server.Close()

	_, err := registry.FetchCoinGeckoCatalog(context.Background(), server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding catalog")
}

// --- FetchCoinGeckoCatalog: cancelled context returns error ---

func TestFetchCoinGeckoCatalog_CancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never responds — will be cancelled first.
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := registry.FetchCoinGeckoCatalog(ctx, server.URL)
	require.Error(t, err)
}
