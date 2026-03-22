package app_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReloader_UpdatesLogLevel(t *testing.T) {
	dir := t.TempDir()
	yaml := `
server:
  port: 8080
telemetry:
  logs:
    enabled: true
    level: "debug"
    format: "json"
`
	err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644)
	require.NoError(t, err)

	levelVar := &slog.LevelVar{}
	levelVar.Set(slog.LevelInfo)

	reloader := app.NewReloader(dir, levelVar)
	err = reloader.Reload()
	require.NoError(t, err)

	assert.Equal(t, slog.LevelDebug, levelVar.Level())
}

func TestReloader_InvalidConfig(t *testing.T) {
	reloader := app.NewReloader("/nonexistent/path", nil)
	err := reloader.Reload()
	assert.Error(t, err)
}
