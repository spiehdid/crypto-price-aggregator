package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type Config struct {
	TracesEnabled  bool
	TracesExporter string // "stdout" or "otlp"
	SampleRate     float64

	MetricsEnabled  bool
	MetricsExporter string // "prometheus" or "stdout"
	MetricsPort     int
}

type Shutdown func(context.Context) error

func Setup(ctx context.Context, cfg Config) (Shutdown, error) {
	var shutdowns []func(context.Context) error

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("crypto-price-aggregator"),
			semconv.ServiceVersion("0.1.0"),
		),
	)
	if err != nil {
		return noopShutdown, fmt.Errorf("creating resource: %w", err)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if cfg.TracesEnabled {
		tp, shutdown, err := setupTraces(ctx, res, cfg)
		if err != nil {
			return noopShutdown, fmt.Errorf("setting up traces: %w", err)
		}
		otel.SetTracerProvider(tp)
		shutdowns = append(shutdowns, shutdown)
		slog.Info("telemetry: traces enabled", "exporter", cfg.TracesExporter)
	}

	if cfg.MetricsEnabled {
		mp, shutdown, err := setupMetrics(ctx, res, cfg)
		if err != nil {
			return noopShutdown, fmt.Errorf("setting up metrics: %w", err)
		}
		otel.SetMeterProvider(mp)
		shutdowns = append(shutdowns, shutdown)
		slog.Info("telemetry: metrics enabled", "exporter", cfg.MetricsExporter)

		if cfg.MetricsExporter == "prometheus" && cfg.MetricsPort > 0 {
			go func() {
				mux := http.NewServeMux()
				mux.Handle("/metrics", promhttp.Handler())
				addr := fmt.Sprintf(":%d", cfg.MetricsPort)
				slog.Info("prometheus metrics server listening", "addr", addr)
				if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
					slog.Error("metrics server error", "error", err)
				}
			}()
		}
	}

	return func(ctx context.Context) error {
		var firstErr error
		for _, fn := range shutdowns {
			if err := fn(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}, nil
}

func setupTraces(ctx context.Context, res *resource.Resource, cfg Config) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	var exporter sdktrace.SpanExporter
	var err error

	switch cfg.TracesExporter {
	case "stdout":
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	default:
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
	if err != nil {
		return nil, nil, fmt.Errorf("creating trace exporter: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRate))

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	return tp, tp.Shutdown, nil
}

func setupMetrics(ctx context.Context, res *resource.Resource, cfg Config) (*sdkmetric.MeterProvider, func(context.Context) error, error) {
	switch cfg.MetricsExporter {
	case "prometheus":
		promExporter, err := prometheus.New()
		if err != nil {
			return nil, nil, fmt.Errorf("creating prometheus exporter: %w", err)
		}
		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(promExporter),
			sdkmetric.WithResource(res),
		)
		return mp, mp.Shutdown, nil

	case "stdout":
		exporter, err := stdoutmetric.New()
		if err != nil {
			return nil, nil, fmt.Errorf("creating stdout metric exporter: %w", err)
		}
		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(10*time.Second))),
			sdkmetric.WithResource(res),
		)
		return mp, mp.Shutdown, nil

	default:
		return nil, nil, fmt.Errorf("unknown metrics exporter: %s", cfg.MetricsExporter)
	}
}

func noopShutdown(_ context.Context) error {
	return nil
}
