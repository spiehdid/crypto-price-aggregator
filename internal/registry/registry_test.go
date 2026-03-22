package registry_test

import (
	"testing"

	"github.com/spiehdid/crypto-price-aggregator/internal/registry"
	"github.com/stretchr/testify/assert"
)

func TestRegistry_Resolve(t *testing.T) {
	r := registry.New()
	r.Add("ethereum", "0xdac17f958d2ee523a2206206994597c13d831ec7", "tether")

	coinID, found := r.Resolve("ethereum", "0xdac17f958d2ee523a2206206994597c13d831ec7")
	assert.True(t, found)
	assert.Equal(t, "tether", coinID)
}

func TestRegistry_ResolveNormalizesCase(t *testing.T) {
	r := registry.New()
	r.Add("ethereum", "0xdAC17F958D2ee523a2206206994597C13D831ec7", "tether")

	coinID, found := r.Resolve("ethereum", "0xdac17f958d2ee523a2206206994597c13d831ec7")
	assert.True(t, found)
	assert.Equal(t, "tether", coinID)
}

func TestRegistry_ResolveNotFound(t *testing.T) {
	r := registry.New()
	_, found := r.Resolve("ethereum", "0x0000")
	assert.False(t, found)
}

func TestRegistry_Contracts(t *testing.T) {
	r := registry.New()
	r.Add("ethereum", "0xabc", "tether")
	r.Add("tron", "TR7NH", "tether")

	contracts := r.Contracts("tether")
	assert.Len(t, contracts, 2)
}

func TestRegistry_LoadCatalog(t *testing.T) {
	r := registry.New()
	catalog := []registry.CoinEntry{
		{
			ID: "tether", Symbol: "usdt",
			Platforms: map[string]string{
				"ethereum": "0xdac17f958d2ee523a2206206994597c13d831ec7",
				"tron":     "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
				"solana":   "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB",
			},
		},
		{
			ID: "usd-coin", Symbol: "usdc",
			Platforms: map[string]string{
				"ethereum": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
			},
		},
	}

	chains := map[string]bool{"ethereum": true, "tron": true}
	r.LoadCatalog(catalog, chains)

	coinID, found := r.Resolve("ethereum", "0xdac17f958d2ee523a2206206994597c13d831ec7")
	assert.True(t, found)
	assert.Equal(t, "tether", coinID)

	coinID, found = r.Resolve("tron", "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t")
	assert.True(t, found)
	assert.Equal(t, "tether", coinID)

	// Solana not in allowed chains
	_, found = r.Resolve("solana", "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB")
	assert.False(t, found)

	coinID, found = r.Resolve("ethereum", "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48")
	assert.True(t, found)
	assert.Equal(t, "usd-coin", coinID)
}

func TestRegistry_Count(t *testing.T) {
	r := registry.New()
	r.Add("ethereum", "0xabc", "tether")
	r.Add("ethereum", "0xdef", "usdc")
	assert.Equal(t, 2, r.Count())
}
