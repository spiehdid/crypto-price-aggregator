package telemetry_test

import (
	"context"
	"testing"

	"github.com/spiehdid/crypto-price-aggregator/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetup_AllDisabled(t *testing.T) {
	shutdown, err := telemetry.Setup(context.Background(), telemetry.Config{
		TracesEnabled:  false,
		MetricsEnabled: false,
	})
	require.NoError(t, err)
	assert.NoError(t, shutdown(context.Background()))
}

func TestSetup_TracesEnabled_Stdout(t *testing.T) {
	shutdown, err := telemetry.Setup(context.Background(), telemetry.Config{
		TracesEnabled:  true,
		TracesExporter: "stdout",
		SampleRate:     1.0,
	})
	require.NoError(t, err)
	assert.NoError(t, shutdown(context.Background()))
}

func TestSetup_MetricsEnabled_Prometheus(t *testing.T) {
	shutdown, err := telemetry.Setup(context.Background(), telemetry.Config{
		MetricsEnabled:  true,
		MetricsExporter: "prometheus",
		MetricsPort:     0, // don't start HTTP server in test
	})
	require.NoError(t, err)
	assert.NoError(t, shutdown(context.Background()))
}

func TestSetup_MetricsEnabled_Stdout(t *testing.T) {
	shutdown, err := telemetry.Setup(context.Background(), telemetry.Config{
		MetricsEnabled:  true,
		MetricsExporter: "stdout",
	})
	require.NoError(t, err)
	assert.NoError(t, shutdown(context.Background()))
}

func TestSetup_MetricsEnabled_UnknownExporter(t *testing.T) {
	_, err := telemetry.Setup(context.Background(), telemetry.Config{
		MetricsEnabled:  true,
		MetricsExporter: "unknown-exporter",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown metrics exporter")
}

func TestSetup_TracesAndMetrics_BothEnabled(t *testing.T) {
	shutdown, err := telemetry.Setup(context.Background(), telemetry.Config{
		TracesEnabled:   true,
		TracesExporter:  "stdout",
		SampleRate:      0.5,
		MetricsEnabled:  true,
		MetricsExporter: "stdout",
	})
	require.NoError(t, err)
	assert.NoError(t, shutdown(context.Background()))
}
