package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig              `mapstructure:"server"`
	Pricing   PricingConfig             `mapstructure:"pricing"`
	Polling   PollingConfig             `mapstructure:"polling"`
	Storage   StorageConfig             `mapstructure:"storage"`
	Routing   RoutingConfig             `mapstructure:"routing"`
	Providers map[string]ProviderConfig `mapstructure:"providers"`
	Registry  RegistryConfig            `mapstructure:"registry"`
	Telemetry TelemetryConfig           `mapstructure:"telemetry"`
}

type RegistryConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	RefreshInterval time.Duration `mapstructure:"refresh_interval"`
	Chains          []string      `mapstructure:"chains"`
}

type ServerConfig struct {
	Host         string          `mapstructure:"host"`
	Port         int             `mapstructure:"port"`
	ReadTimeout  time.Duration   `mapstructure:"read_timeout"`
	WriteTimeout time.Duration   `mapstructure:"write_timeout"`
	AdminAPIKey  string          `mapstructure:"admin_api_key"`
	RateLimit    RateLimitConfig `mapstructure:"rate_limit"`
	CORS         CORSConfig      `mapstructure:"cors"`
	ForceHTTPS   bool            `mapstructure:"force_https"`
}

type RateLimitConfig struct {
	Enabled           bool    `mapstructure:"enabled"`
	RequestsPerSecond float64 `mapstructure:"requests_per_second"`
	Burst             int     `mapstructure:"burst"`
}

type CORSConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

type PricingConfig struct {
	DefaultCurrency string           `mapstructure:"default_currency"`
	CacheTTL        time.Duration    `mapstructure:"cache_ttl"`
	MaxRetries      int              `mapstructure:"max_retries"`
	StoreHistory    bool             `mapstructure:"store_history"`
	Validation      ValidationConfig `mapstructure:"validation"`
}

type ValidationConfig struct {
	Enabled               bool          `mapstructure:"enabled"`
	MaxStaleness          time.Duration `mapstructure:"max_staleness"`
	MaxDeviationPercent   float64       `mapstructure:"max_deviation_percent"`
	ConsensusProviders    int           `mapstructure:"consensus_providers"`
	ConsensusMinProviders int           `mapstructure:"consensus_min_providers"`
	ConsensusTimeout      time.Duration `mapstructure:"consensus_timeout"`
	ResponseMode          string        `mapstructure:"response_mode"` // single | validated | consensus
	StablecoinDeviation   float64       `mapstructure:"stablecoin_deviation"`
	MajorCoinDeviation    float64       `mapstructure:"major_coin_deviation"`
	AltcoinDeviation      float64       `mapstructure:"altcoin_deviation"`
}

type PollingConfig struct {
	DefaultInterval time.Duration `mapstructure:"default_interval"`
	MinInterval     time.Duration `mapstructure:"min_interval"`
}

type StorageConfig struct {
	Postgres   PostgresConfig   `mapstructure:"postgres"`
	Redis      RedisConfig      `mapstructure:"redis"`
	ClickHouse ClickHouseConfig `mapstructure:"clickhouse"`
}

type ClickHouseConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	Addr          string        `mapstructure:"addr"`
	Database      string        `mapstructure:"database"`
	Username      string        `mapstructure:"username"`
	Password      string        `mapstructure:"password"`
	BatchSize     int           `mapstructure:"batch_size"`
	BatchInterval time.Duration `mapstructure:"batch_interval"`
}

type PostgresConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	DSN      string `mapstructure:"dsn"`
	MaxConns int32  `mapstructure:"max_conns"`
	MinConns int32  `mapstructure:"min_conns"`
}

type RedisConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type RoutingConfig struct {
	Strategy      string        `mapstructure:"strategy"`
	AllowPaid     bool          `mapstructure:"allow_paid"`
	MonthlyBudget float64       `mapstructure:"monthly_budget_usd"`
	Weights       WeightsConfig `mapstructure:"weights"`
}

type WeightsConfig struct {
	RateRemaining float64 `mapstructure:"rate_remaining"`
	Latency       float64 `mapstructure:"latency"`
	Cost          float64 `mapstructure:"cost"`
}

type ProviderConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	Tier           string   `mapstructure:"tier"`
	Priority       int      `mapstructure:"priority"`
	RateLimit      int      `mapstructure:"rate_limit"`
	APIKey         string   `mapstructure:"api_key"`
	BaseURL        string   `mapstructure:"base_url"`
	WSURL          string   `mapstructure:"ws_url"`
	CircuitBreaker CBConfig `mapstructure:"circuit_breaker"`
}

type CBConfig struct {
	FailureThreshold int           `mapstructure:"failure_threshold"`
	RecoveryTimeout  time.Duration `mapstructure:"recovery_timeout"`
}

type TelemetryConfig struct {
	Logs    LogsConfig    `mapstructure:"logs"`
	Metrics MetricsConfig `mapstructure:"metrics"`
	Traces  TracesConfig  `mapstructure:"traces"`
}

type LogsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Level   string `mapstructure:"level"`
	Format  string `mapstructure:"format"`
}

type MetricsConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Exporter string `mapstructure:"exporter"`
	Port     int    `mapstructure:"port"`
}

type TracesConfig struct {
	Enabled    bool    `mapstructure:"enabled"`
	Exporter   string  `mapstructure:"exporter"`
	Endpoint   string  `mapstructure:"endpoint"`
	SampleRate float64 `mapstructure:"sample_rate"`
}

func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.Server.ReadTimeout <= 0 {
		return fmt.Errorf("server.read_timeout must be positive")
	}
	if c.Server.WriteTimeout <= 0 {
		return fmt.Errorf("server.write_timeout must be positive")
	}
	if c.Pricing.CacheTTL <= 0 {
		return fmt.Errorf("pricing.cache_ttl must be positive")
	}
	if c.Pricing.MaxRetries <= 0 {
		c.Pricing.MaxRetries = 1 // auto-correct
	}
	if c.Server.RateLimit.Enabled {
		if c.Server.RateLimit.RequestsPerSecond <= 0 {
			return fmt.Errorf("rate_limit.requests_per_second must be positive")
		}
		if c.Server.RateLimit.Burst <= 0 {
			return fmt.Errorf("rate_limit.burst must be positive")
		}
	}
	if c.Server.AdminAPIKey == "" {
		slog.Warn("admin API key not set — admin endpoints are unprotected")
	}
	return nil
}

func Load(configPath string) (*Config, error) {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(configPath)

	v.SetEnvPrefix("CPA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "5s")
	v.SetDefault("server.write_timeout", "10s")
	v.SetDefault("server.admin_api_key", "")
	v.SetDefault("server.rate_limit.enabled", false)
	v.SetDefault("server.rate_limit.requests_per_second", 10)
	v.SetDefault("server.rate_limit.burst", 20)
	v.SetDefault("server.cors.enabled", false)
	v.SetDefault("server.cors.allowed_origins", []string{"*"})
	v.SetDefault("server.force_https", false)

	v.SetDefault("pricing.default_currency", "usd")
	v.SetDefault("pricing.cache_ttl", "30s")
	v.SetDefault("pricing.max_retries", 3)
	v.SetDefault("pricing.store_history", true)

	v.SetDefault("pricing.validation.enabled", true)
	v.SetDefault("pricing.validation.max_staleness", "5m")
	v.SetDefault("pricing.validation.max_deviation_percent", 5.0)
	v.SetDefault("pricing.validation.consensus_providers", 3)
	v.SetDefault("pricing.validation.consensus_min_providers", 2)
	v.SetDefault("pricing.validation.consensus_timeout", "3s")
	v.SetDefault("pricing.validation.response_mode", "validated")
	v.SetDefault("pricing.validation.stablecoin_deviation", 2.0)
	v.SetDefault("pricing.validation.major_coin_deviation", 10.0)
	v.SetDefault("pricing.validation.altcoin_deviation", 20.0)

	v.SetDefault("polling.default_interval", "60s")
	v.SetDefault("polling.min_interval", "10s")

	v.SetDefault("routing.strategy", "smart")
	v.SetDefault("routing.allow_paid", false)
	v.SetDefault("routing.monthly_budget_usd", 0)
	v.SetDefault("routing.weights.rate_remaining", 0.4)
	v.SetDefault("routing.weights.latency", 0.3)
	v.SetDefault("routing.weights.cost", 0.3)

	v.SetDefault("storage.clickhouse.enabled", false)
	v.SetDefault("storage.clickhouse.addr", "localhost:9000")
	v.SetDefault("storage.clickhouse.database", "cpa")
	v.SetDefault("storage.clickhouse.username", "default")
	v.SetDefault("storage.clickhouse.password", "")
	v.SetDefault("storage.clickhouse.batch_size", 1000)
	v.SetDefault("storage.clickhouse.batch_interval", "5s")

	v.SetDefault("registry.enabled", true)
	v.SetDefault("registry.refresh_interval", "1h")
	v.SetDefault("registry.chains", []string{"ethereum", "binance-smart-chain", "polygon-pos", "arbitrum-one", "optimistic-ethereum", "avalanche", "base", "solana", "tron"})

	v.SetDefault("telemetry.logs.enabled", true)
	v.SetDefault("telemetry.logs.level", "info")
	v.SetDefault("telemetry.logs.format", "json")
}
