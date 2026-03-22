package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spiehdid/crypto-price-aggregator/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultConfig(t *testing.T) {
	dir := t.TempDir()
	yaml := `
server:
  host: "0.0.0.0"
  port: 8080
pricing:
  default_currency: "usd"
  cache_ttl: "30s"
  max_retries: 3
providers:
  coingecko:
    enabled: true
    tier: "free"
    rate_limit: 30
    base_url: "https://api.coingecko.com/api/v3"
`
	err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644)
	require.NoError(t, err)

	cfg, err := config.Load(dir)
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "usd", cfg.Pricing.DefaultCurrency)
	assert.Equal(t, 3, cfg.Pricing.MaxRetries)
	assert.True(t, cfg.Providers["coingecko"].Enabled)
}

func TestLoad_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	yaml := `
server:
  port: 8080
`
	err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644)
	require.NoError(t, err)

	t.Setenv("CPA_SERVER_PORT", "9090")

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Server.Port)
}
