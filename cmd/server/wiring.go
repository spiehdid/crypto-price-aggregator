package main

import (
	"database/sql"
	"log/slog"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/alternativeme"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/bingx"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/bitfinex"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/bithumb"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/bitstamp"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/blockchain"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/btse"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/bybit"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coinbase"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coincap"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coincodex"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coingecko"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coinlore"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coinmarketcap"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coinpaprika"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/coinranking"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/cryptocompare"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/cryptodotcom"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/cryptorank"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/dexscreener"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/dia"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/dydx"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/freecryptoapi"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/gateio"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/gemini"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/htx"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/kraken"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/kucoin"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/lbank"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/livecoinwatch"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/messari"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/mexc"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/okx"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/phemex"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/poloniex"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/twelvedata"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/upbit"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/wazirx"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/woox"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/xtcom"
	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/config"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
	"github.com/spiehdid/crypto-price-aggregator/internal/router"
)

type multiListener struct {
	listeners []app.PriceListener
}

func (m *multiListener) OnPriceUpdate(price *model.Price) {
	for _, l := range m.listeners {
		l.OnPriceUpdate(price)
	}
}

func buildProviderRegistrations(cfg *config.Config) []router.ProviderRegistration {
	var regs []router.ProviderRegistration

	for name, pcfg := range cfg.Providers {
		if !pcfg.Enabled {
			continue
		}

		provider := createProvider(name, pcfg)
		if provider == nil {
			slog.Warn("unknown provider, skipping", "name", name)
			continue
		}

		tier := model.TierFree
		if pcfg.Tier == "paid" {
			tier = model.TierPaid
		}

		cbThreshold := pcfg.CircuitBreaker.FailureThreshold
		if cbThreshold <= 0 {
			cbThreshold = 5
		}
		cbTimeout := pcfg.CircuitBreaker.RecoveryTimeout
		if cbTimeout <= 0 {
			cbTimeout = 60 * time.Second
		}

		regs = append(regs, router.ProviderRegistration{
			Provider:    provider,
			Tier:        tier,
			Priority:    pcfg.Priority,
			RateLimit:   pcfg.RateLimit,
			RateWindow:  time.Minute,
			CBThreshold: cbThreshold,
			CBTimeout:   cbTimeout,
			CostPerCall: decimal.Zero,
		})

		slog.Info("provider registered", "name", name, "tier", pcfg.Tier, "priority", pcfg.Priority)
	}

	return regs
}

func createProvider(name string, pcfg config.ProviderConfig) port.PriceProvider {
	switch name {
	case "coingecko":
		return coingecko.New(coingecko.Config{
			BaseURL:   pcfg.BaseURL,
			APIKey:    pcfg.APIKey,
			RateLimit: pcfg.RateLimit,
		})
	case "coinmarketcap":
		return coinmarketcap.New(coinmarketcap.Config{
			BaseURL:   pcfg.BaseURL,
			APIKey:    pcfg.APIKey,
			RateLimit: pcfg.RateLimit,
		})
	case "coinpaprika":
		return coinpaprika.New(coinpaprika.Config{
			BaseURL:   pcfg.BaseURL,
			RateLimit: pcfg.RateLimit,
		})
	case "cryptocompare":
		return cryptocompare.New(cryptocompare.Config{
			BaseURL:   pcfg.BaseURL,
			RateLimit: pcfg.RateLimit,
		})
	case "coincap":
		return coincap.New(coincap.Config{BaseURL: pcfg.BaseURL})
	case "coinlore":
		return coinlore.New(coinlore.Config{BaseURL: pcfg.BaseURL})
	case "coinranking":
		return coinranking.New(coinranking.Config{BaseURL: pcfg.BaseURL, APIKey: pcfg.APIKey})
	case "kucoin":
		return kucoin.New(kucoin.Config{BaseURL: pcfg.BaseURL})
	case "kraken":
		return kraken.New(kraken.Config{BaseURL: pcfg.BaseURL})
	case "bitfinex":
		return bitfinex.New(bitfinex.Config{BaseURL: pcfg.BaseURL})
	case "okx":
		return okx.New(okx.Config{BaseURL: pcfg.BaseURL})
	case "bybit":
		return bybit.New(bybit.Config{BaseURL: pcfg.BaseURL})
	case "mexc":
		return mexc.New(mexc.Config{BaseURL: pcfg.BaseURL})
	case "gateio":
		return gateio.New(gateio.Config{BaseURL: pcfg.BaseURL})
	case "htx":
		return htx.New(htx.Config{BaseURL: pcfg.BaseURL})
	case "blockchain":
		return blockchain.New(blockchain.Config{BaseURL: pcfg.BaseURL})
	case "messari":
		return messari.New(messari.Config{BaseURL: pcfg.BaseURL, APIKey: pcfg.APIKey})
	case "livecoinwatch":
		return livecoinwatch.New(livecoinwatch.Config{BaseURL: pcfg.BaseURL, APIKey: pcfg.APIKey})
	case "dia":
		return dia.New(dia.Config{BaseURL: pcfg.BaseURL})
	case "twelvedata":
		return twelvedata.New(twelvedata.Config{BaseURL: pcfg.BaseURL, APIKey: pcfg.APIKey})
	case "freecryptoapi":
		return freecryptoapi.New(freecryptoapi.Config{BaseURL: pcfg.BaseURL, APIKey: pcfg.APIKey})
	case "coincodex":
		return coincodex.New(coincodex.Config{BaseURL: pcfg.BaseURL})
	case "coinbase":
		return coinbase.New(coinbase.Config{BaseURL: pcfg.BaseURL})
	case "gemini":
		return gemini.New(gemini.Config{BaseURL: pcfg.BaseURL})
	case "cryptodotcom":
		return cryptodotcom.New(cryptodotcom.Config{BaseURL: pcfg.BaseURL})
	case "poloniex":
		return poloniex.New(poloniex.Config{BaseURL: pcfg.BaseURL})
	case "btse":
		return btse.New(btse.Config{BaseURL: pcfg.BaseURL})
	case "xtcom":
		return xtcom.New(xtcom.Config{BaseURL: pcfg.BaseURL})
	case "lbank":
		return lbank.New(lbank.Config{BaseURL: pcfg.BaseURL})
	case "bingx":
		return bingx.New(bingx.Config{BaseURL: pcfg.BaseURL})
	case "alternativeme":
		return alternativeme.New(alternativeme.Config{BaseURL: pcfg.BaseURL})
	case "wazirx":
		return wazirx.New(wazirx.Config{BaseURL: pcfg.BaseURL})
	case "cryptorank":
		return cryptorank.New(cryptorank.Config{BaseURL: pcfg.BaseURL, APIKey: pcfg.APIKey})
	case "upbit":
		return upbit.New(upbit.Config{BaseURL: pcfg.BaseURL})
	case "bithumb":
		return bithumb.New(bithumb.Config{BaseURL: pcfg.BaseURL})
	case "woox":
		return woox.New(woox.Config{BaseURL: pcfg.BaseURL})
	case "phemex":
		return phemex.New(phemex.Config{BaseURL: pcfg.BaseURL})
	case "dexscreener":
		return dexscreener.New(dexscreener.Config{BaseURL: pcfg.BaseURL})
	case "bitstamp":
		return bitstamp.New(bitstamp.Config{BaseURL: pcfg.BaseURL})
	case "dydx":
		return dydx.New(dydx.Config{BaseURL: pcfg.BaseURL})
	default:
		return nil
	}
}

func buildStrategy(rcfg config.RoutingConfig) router.Strategy {
	switch rcfg.Strategy {
	case "priority":
		return router.NewPriorityStrategy(rcfg.AllowPaid)
	case "roundrobin":
		return router.NewRoundRobinStrategy(rcfg.AllowPaid)
	default: // "smart"
		return router.NewSmartStrategy(router.SmartWeights{
			RateRemaining: rcfg.Weights.RateRemaining,
			Latency:       rcfg.Weights.Latency,
			Cost:          rcfg.Weights.Cost,
		}, rcfg.AllowPaid)
	}
}

func setupLogger(cfg config.LogsConfig) *slog.LevelVar {
	levelVar := &slog.LevelVar{}
	switch cfg.Level {
	case "debug":
		levelVar.Set(slog.LevelDebug)
	case "warn":
		levelVar.Set(slog.LevelWarn)
	case "error":
		levelVar.Set(slog.LevelError)
	default:
		levelVar.Set(slog.LevelInfo)
	}

	opts := &slog.HandlerOptions{Level: levelVar}

	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
	return levelVar
}

func runMigrations(dsn string) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		slog.Error("failed to open db for migrations", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	driver, err := migratepg.WithInstance(db, &migratepg.Config{})
	if err != nil {
		slog.Error("failed to create migration driver", "error", err)
		os.Exit(1)
	}

	m, err := migrate.NewWithDatabaseInstance("file://migrations", "postgres", driver)
	if err != nil {
		slog.Error("failed to create migrator", "error", err)
		os.Exit(1)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}

	slog.Info("database migrations applied")
}
