# Contributing to Crypto Price Aggregator

## Development Setup

1. Install Go 1.22+
2. Clone the repo
3. Run `make build` to verify everything compiles
4. Run `make test` to run the test suite

## Running Locally

```bash
# Without Docker (in-memory storage)
make run

# With Docker (Postgres + Redis + ClickHouse)
docker-compose up -d
CPA_STORAGE_POSTGRES_ENABLED=true CPA_STORAGE_REDIS_ENABLED=true make run
```

## Adding a New Provider

1. Create a new directory: `internal/adapter/provider/<name>/`
2. Create `<name>.go` implementing `port.PriceProvider`:
   - Embed `common.BaseProvider` for shared functionality
   - Implement `GetPrice()` and `GetPrices()`
   - Use `b.DoRequest()` for HTTP calls (handles errors, rate limits)
   - Use `b.DecodeJSON()` for response parsing (preserves decimal precision)
3. Create `<name>_test.go` with tests using `httptest.NewServer`
4. Register the provider in `cmd/server/wiring.go` (`createProvider` function)
5. Add config section in `config/config.yaml`

## Code Style

- Follow standard Go conventions
- Run `make lint` before submitting
- Run `make test-race` to check for race conditions
- Keep files focused — split when >300 lines

## Testing

- Write tests BEFORE implementation (TDD)
- Use `httptest.NewServer` for HTTP mocks
- Use `gomock` for interface mocks
- Use build tags for integration tests: `//go:build integration`
- Run `make test-race` to detect race conditions

## Pull Requests

- One feature per PR
- Include tests
- Update docs if API changes
- Run `make lint && make test` before submitting
