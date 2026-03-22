package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	chstore "github.com/spiehdid/crypto-price-aggregator/internal/adapter/storage/clickhouse"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/storage/memory"
	pgstore "github.com/spiehdid/crypto-price-aggregator/internal/adapter/storage/postgres"
	rediscache "github.com/spiehdid/crypto-price-aggregator/internal/adapter/storage/redis"
	binanceStream "github.com/spiehdid/crypto-price-aggregator/internal/adapter/stream/binance"
	httpTransport "github.com/spiehdid/crypto-price-aggregator/internal/adapter/transport/http"
	wsTransport "github.com/spiehdid/crypto-price-aggregator/internal/adapter/transport/ws"
	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/config"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
	"github.com/spiehdid/crypto-price-aggregator/internal/registry"
	"github.com/spiehdid/crypto-price-aggregator/internal/router"
	"github.com/spiehdid/crypto-price-aggregator/internal/telemetry"

	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coingecko"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/dexscreener"
)

func main() {
	cfg, err := config.Load("config")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid config", "error", err)
		os.Exit(1)
	}

	levelVar := setupLogger(cfg.Telemetry.Logs)

	slog.Info("starting crypto-price-aggregator",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
	)

	ctx := context.Background()

	otelShutdown, err := telemetry.Setup(ctx, telemetry.Config{
		TracesEnabled:   cfg.Telemetry.Traces.Enabled,
		TracesExporter:  cfg.Telemetry.Traces.Exporter,
		SampleRate:      cfg.Telemetry.Traces.SampleRate,
		MetricsEnabled:  cfg.Telemetry.Metrics.Enabled,
		MetricsExporter: cfg.Telemetry.Metrics.Exporter,
		MetricsPort:     cfg.Telemetry.Metrics.Port,
	})
	if err != nil {
		slog.Error("failed to setup telemetry", "error", err)
		os.Exit(1)
	}

	registrations := buildProviderRegistrations(cfg)
	slog.Info("providers registered", "count", len(registrations))

	strategy := buildStrategy(cfg.Routing)
	providerRouter := router.NewMultiRouter(strategy, registrations)

	var store port.PriceStore
	var pgStore *pgstore.Store
	if cfg.Storage.Postgres.Enabled {
		runMigrations(cfg.Storage.Postgres.DSN)
		pgStore, err = pgstore.NewStore(ctx, cfg.Storage.Postgres.DSN)
		if err != nil {
			slog.Error("failed to connect to postgres", "error", err)
			os.Exit(1)
		}
		store = pgStore
		slog.Info("storage: postgresql connected")
	} else {
		store = memory.NewStore()
		slog.Info("storage: in-memory (postgresql disabled)")
	}

	var cache port.Cache
	var redisCache *rediscache.Cache
	if cfg.Storage.Redis.Enabled {
		redisCache, err = rediscache.NewCache(ctx, rediscache.Config{
			Addr:     cfg.Storage.Redis.Addr,
			Password: cfg.Storage.Redis.Password,
			DB:       cfg.Storage.Redis.DB,
		})
		if err != nil {
			slog.Error("failed to connect to redis", "error", err)
			os.Exit(1)
		}
		cache = redisCache
		slog.Info("cache: redis connected")
	} else {
		cache = memory.NewCache()
		slog.Info("cache: in-memory (redis disabled)")
	}

	var analyticsSvc *app.AnalyticsService
	if cfg.Storage.ClickHouse.Enabled {
		chStore, err := chstore.NewStore(ctx, chstore.Config{
			Addr:          cfg.Storage.ClickHouse.Addr,
			Database:      cfg.Storage.ClickHouse.Database,
			Username:      cfg.Storage.ClickHouse.Username,
			Password:      cfg.Storage.ClickHouse.Password,
			BatchSize:     cfg.Storage.ClickHouse.BatchSize,
			BatchInterval: cfg.Storage.ClickHouse.BatchInterval,
		})
		if err != nil {
			slog.Error("failed to connect to clickhouse", "error", err)
			os.Exit(1)
		}
		defer chStore.Close()
		store = chStore
		analyticsSvc = app.NewAnalyticsService(chstore.NewAnalytics(chStore.Conn()))
		slog.Info("clickhouse connected", "addr", cfg.Storage.ClickHouse.Addr)
	}

	wsHub := wsTransport.NewHub()
	go wsHub.Run()

	alertSvc := app.NewAlertService()

	var tokenRegistry *registry.TokenRegistry
	var tokenProviders []port.TokenPriceProvider

	if cfg.Registry.Enabled {
		tokenRegistry = registry.New()

		chains := make(map[string]bool)
		for _, c := range cfg.Registry.Chains {
			chains[c] = true
		}

		cgBaseURL := "https://api.coingecko.com"
		if provCfg, ok := cfg.Providers["coingecko"]; ok && provCfg.BaseURL != "" {
			cgBaseURL = provCfg.BaseURL
		}

		entries, err := registry.FetchCoinGeckoCatalog(ctx, cgBaseURL)
		if err != nil {
			slog.Warn("failed to fetch token catalog", "error", err)
		} else {
			tokenRegistry.LoadCatalog(entries, chains)
			slog.Info("token registry loaded", "tokens", tokenRegistry.Count())
		}

		registry.StartRefresh(ctx, tokenRegistry, cgBaseURL, chains, cfg.Registry.RefreshInterval)

		tokenProviders = append(tokenProviders, coingecko.NewTokenProvider(cgBaseURL))
		if provCfg, ok := cfg.Providers["dexscreener"]; ok && provCfg.BaseURL != "" {
			tokenProviders = append(tokenProviders, dexscreener.NewTokenProvider(provCfg.BaseURL))
		}
	}

	listener := &multiListener{listeners: []app.PriceListener{wsHub, alertSvc}}

	validator := app.NewPriceValidator(app.PriceValidatorConfig{
		Enabled:               cfg.Pricing.Validation.Enabled,
		MaxStaleness:          cfg.Pricing.Validation.MaxStaleness,
		MaxDeviationFromCache: cfg.Pricing.Validation.MaxDeviationPercent,
		StablecoinDeviation:   cfg.Pricing.Validation.StablecoinDeviation,
		MajorCoinDeviation:    cfg.Pricing.Validation.MajorCoinDeviation,
		AltcoinDeviation:      cfg.Pricing.Validation.AltcoinDeviation,
	})

	outlier := app.NewOutlierDetector(cfg.Pricing.Validation.MaxDeviationPercent)

	consensus := app.NewConsensusEngine(app.ConsensusConfig{
		Enabled:      cfg.Pricing.Validation.ResponseMode == "consensus",
		MinProviders: cfg.Pricing.Validation.ConsensusMinProviders,
		MaxDeviation: cfg.Pricing.Validation.MaxDeviationPercent,
		Timeout:      cfg.Pricing.Validation.ConsensusTimeout,
	})

	priceSvc := app.NewPriceService(app.PriceServiceDeps{
		Router:         providerRouter,
		Cache:          cache,
		Store:          store,
		CacheTTL:       cfg.Pricing.CacheTTL,
		MaxRetries:     cfg.Pricing.MaxRetries,
		Listener:       listener,
		Validator:      validator,
		Outlier:        outlier,
		Consensus:      consensus,
		ResponseMode:   cfg.Pricing.Validation.ResponseMode,
		Registry:       tokenRegistry,
		TokenProviders: tokenProviders,
	})

	var binanceProvider *binanceStream.Provider
	if binanceCfg, ok := cfg.Providers["binance"]; ok && binanceCfg.Enabled {
		binanceProvider = binanceStream.NewProvider(binanceStream.Config{
			WSURL: binanceCfg.WSURL,
		})
		go func() {
			coins := []string{"bitcoin", "ethereum", "solana", "cardano", "ripple"}
			ch, err := binanceProvider.Subscribe(ctx, coins, cfg.Pricing.DefaultCurrency)
			if err != nil {
				slog.Error("binance ws subscribe failed", "error", err)
				return
			}
			slog.Info("binance ws stream started", "coins", coins)
			for price := range ch {
				if cache != nil {
					_ = cache.Set(ctx, &price, cfg.Pricing.CacheTTL)
				}
				if store != nil {
					_ = store.Save(ctx, &price)
				}
				wsHub.OnPriceUpdate(&price)
			}
			slog.Warn("binance ws stream ended")
		}()
	}

	subSvc := app.NewSubscriptionService(ctx, priceSvc, cfg.Polling.MinInterval)
	slog.Info("subscription service started", "min_interval", cfg.Polling.MinInterval)

	adminSvc := app.NewAdminService(providerRouter, subSvc, priceSvc, alertSvc)
	handler := httpTransport.NewHandler(priceSvc, adminSvc, alertSvc, analyticsSvc, cfg.Pricing.DefaultCurrency)
	mux := httpTransport.NewRouter(handler, httpTransport.RouterConfig{
		AdminAPIKey:      cfg.Server.AdminAPIKey,
		RateLimitEnabled: cfg.Server.RateLimit.Enabled,
		RateLimitRPS:     cfg.Server.RateLimit.RequestsPerSecond,
		RateLimitBurst:   cfg.Server.RateLimit.Burst,
		WSHandler:        wsTransport.HandleWS(wsHub),
		CORSEnabled:      cfg.Server.CORS.Enabled,
		CORSOrigins:      cfg.Server.CORS.AllowedOrigins,
		ForceHTTPS:       cfg.Server.ForceHTTPS,
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	shutdownSignalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	reloader := app.NewReloader("config", levelVar)
	sighupCh := make(chan os.Signal, 1)
	signal.Notify(sighupCh, syscall.SIGHUP)
	go func() {
		for range sighupCh {
			if err := reloader.Reload(); err != nil {
				slog.Error("config reload failed", "error", err)
			}
		}
	}()

	<-shutdownSignalCtx.Done()
	slog.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}

	if binanceProvider != nil {
		if err := binanceProvider.Close(); err != nil {
			slog.Warn("binance provider close error", "error", err)
		}
		slog.Info("binance provider closed")
	}

	wsHub.Stop()
	slog.Info("ws hub stopped")

	subSvc.Stop()
	slog.Info("subscription service stopped")

	alertSvc.Stop()
	slog.Info("alert service stopped")

	if err := otelShutdown(shutdownCtx); err != nil {
		slog.Error("telemetry shutdown error", "error", err)
	}
	slog.Info("telemetry shut down")

	if pgStore != nil {
		pgStore.Close()
		slog.Info("postgresql connection closed")
	}
	if redisCache != nil {
		_ = redisCache.Close()
		slog.Info("redis connection closed")
	}

	slog.Info("server stopped")
}
