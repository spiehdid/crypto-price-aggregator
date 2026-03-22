# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- 40 cryptocurrency price provider adapters (CoinGecko, CoinMarketCap, Binance, Coinbase, Kraken, and 35 more)
- Smart routing with 3 strategies: Smart (multi-factor scoring), Priority (ordered fallback), RoundRobin
- Circuit breaker per provider with configurable thresholds
- Rate limit tracking per provider with automatic reset
- Provider accuracy tracking and outlier detection
- Price validation with per-category deviation thresholds (stablecoin/major/altcoin)
- Consensus pricing engine — median from multiple providers
- Background polling subscriptions with configurable intervals
- Binance WebSocket streaming for real-time prices
- WebSocket API for clients (`/ws/prices`)
- Price alerts with webhook notifications (3x retry)
- Crypto-to-crypto conversion endpoint
- OHLC candlestick aggregation from price history
- Address-based price lookup with auto-learning token registry
- Pluggable storage: in-memory, PostgreSQL, Redis cache, ClickHouse analytics
- ClickHouse analytics: provider accuracy, price anomalies, volume distribution
- Embedded web dashboard (Alpine.js + Tailwind + Chart.js)
- OpenTelemetry integration (logs, metrics, traces — each toggleable)
- Prometheus metrics for key operations
- REST API with Swagger/OpenAPI documentation
- Admin API for provider management and monitoring
- Docker multi-stage build (~15MB image)
- docker-compose with Postgres + Redis + ClickHouse
- GitHub Actions CI (lint + test + build)
- Graceful shutdown with correct dependency ordering
- Config validation at startup
- CORS and security headers middleware
- API key authentication for admin endpoints
- Per-IP rate limiting with automatic cleanup
- SSRF prevention for webhook URLs
- Kubernetes manifests and HPA

### Security
- Constant-time API key comparison
- Input validation (coinID format, batch limits)
- Error message sanitization (no internal details in API responses)
- Webhook URL validation (blocks private IPs, internal hostnames)
