package clickhouse

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

const createPricesTable = `
CREATE TABLE IF NOT EXISTS prices (
    coin_id     LowCardinality(String),
    currency    LowCardinality(String),
    value       Float64,
    provider    LowCardinality(String),
    market_cap  Float64  DEFAULT 0,
    volume_24h  Float64  DEFAULT 0,
    change_24h  Float64  DEFAULT 0,
    timestamp   DateTime64(3),
    received_at DateTime64(3)
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(received_at)
ORDER BY (coin_id, currency, received_at)
TTL received_at + INTERVAL 2 YEAR
SETTINGS index_granularity = 8192
`

func EnsureSchema(ctx context.Context, conn driver.Conn) error {
	if err := conn.Exec(ctx, createPricesTable); err != nil {
		return fmt.Errorf("creating prices table: %w", err)
	}
	return nil
}
