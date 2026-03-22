package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(ctx context.Context, dsn string) (*Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing dsn: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Save(ctx context.Context, price *model.Price) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO prices (coin_id, currency, value, provider, timestamp, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		price.CoinID, price.Currency, price.Value, price.Provider,
		price.Timestamp, price.ReceivedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting price: %w", err)
	}
	return nil
}

func (s *Store) SaveBatch(ctx context.Context, prices []model.Price) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, p := range prices {
		_, err := tx.Exec(ctx,
			`INSERT INTO prices (coin_id, currency, value, provider, timestamp, received_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			p.CoinID, p.Currency, p.Value, p.Provider, p.Timestamp, p.ReceivedAt,
		)
		if err != nil {
			return fmt.Errorf("inserting price in batch: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (s *Store) GetLatest(ctx context.Context, coinID, currency string) (*model.Price, error) {
	var p model.Price
	var value decimal.Decimal

	err := s.pool.QueryRow(ctx,
		`SELECT coin_id, currency, value, provider, timestamp, received_at
		 FROM prices
		 WHERE coin_id = $1 AND currency = $2
		 ORDER BY received_at DESC
		 LIMIT 1`,
		coinID, currency,
	).Scan(&p.CoinID, &p.Currency, &value, &p.Provider, &p.Timestamp, &p.ReceivedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying latest price: %w", err)
	}

	p.Value = value
	return &p, nil
}

func (s *Store) GetHistory(ctx context.Context, coinID, currency string, from, to time.Time) ([]model.Price, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT coin_id, currency, value, provider, timestamp, received_at
		 FROM prices
		 WHERE coin_id = $1 AND currency = $2
		   AND received_at >= $3 AND received_at < $4
		 ORDER BY received_at ASC`,
		coinID, currency, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("querying history: %w", err)
	}
	defer rows.Close()

	var prices []model.Price
	for rows.Next() {
		var p model.Price
		var value decimal.Decimal
		if err := rows.Scan(&p.CoinID, &p.Currency, &value, &p.Provider, &p.Timestamp, &p.ReceivedAt); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}
		p.Value = value
		prices = append(prices, p)
	}

	return prices, rows.Err()
}

func (s *Store) GetOHLC(ctx context.Context, coinID, currency string, from, to time.Time, interval time.Duration) ([]model.OHLC, error) {
	// Use generate_series + lateral join for time bucketing
	query := `
        WITH buckets AS (
            SELECT generate_series($3::timestamptz, $4::timestamptz - $5::interval, $5::interval) AS bucket_start
        )
        SELECT
            b.bucket_start,
            (SELECT value FROM prices WHERE coin_id = $1 AND currency = $2 AND received_at >= b.bucket_start AND received_at < b.bucket_start + $5::interval ORDER BY received_at ASC LIMIT 1) AS open,
            MAX(p.value) AS high,
            MIN(p.value) AS low,
            (SELECT value FROM prices WHERE coin_id = $1 AND currency = $2 AND received_at >= b.bucket_start AND received_at < b.bucket_start + $5::interval ORDER BY received_at DESC LIMIT 1) AS close
        FROM buckets b
        LEFT JOIN prices p ON p.coin_id = $1 AND p.currency = $2 AND p.received_at >= b.bucket_start AND p.received_at < b.bucket_start + $5::interval
        GROUP BY b.bucket_start
        HAVING COUNT(p.id) > 0
        ORDER BY b.bucket_start`

	rows, err := s.pool.Query(ctx, query, coinID, currency, from, to, fmt.Sprintf("%d seconds", int(interval.Seconds())))
	if err != nil {
		return nil, fmt.Errorf("querying ohlc: %w", err)
	}
	defer rows.Close()

	var result []model.OHLC
	for rows.Next() {
		var o model.OHLC
		var open, high, low, close decimal.Decimal
		if err := rows.Scan(&o.Time, &open, &high, &low, &close); err != nil {
			return nil, fmt.Errorf("scanning ohlc row: %w", err)
		}
		o.Open = open
		o.High = high
		o.Low = low
		o.Close = close
		result = append(result, o)
	}
	return result, rows.Err()
}
