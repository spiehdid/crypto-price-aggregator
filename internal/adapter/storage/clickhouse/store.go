package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type Config struct {
	Addr          string
	Database      string
	Username      string
	Password      string
	BatchSize     int
	BatchInterval time.Duration
}

type Store struct {
	conn   driver.Conn
	buffer *BatchBuffer
}

func NewStore(ctx context.Context, cfg Config) (*Store, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to clickhouse: %w", err)
	}

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging clickhouse: %w", err)
	}

	if err := EnsureSchema(ctx, conn); err != nil {
		return nil, fmt.Errorf("ensuring schema: %w", err)
	}

	s := &Store{conn: conn}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}
	batchInterval := cfg.BatchInterval
	if batchInterval <= 0 {
		batchInterval = 5 * time.Second
	}

	s.buffer = NewBatchBuffer(batchSize, batchInterval, s.flushBatch)
	s.buffer.Start()

	return s, nil
}

func (s *Store) Conn() driver.Conn {
	return s.conn
}

func (s *Store) FlushStats() (flushed, errors int64) {
	return s.buffer.FlushStats()
}

func (s *Store) Close() {
	s.buffer.Stop()
	_ = s.conn.Close()
}

func (s *Store) Save(_ context.Context, price *model.Price) error {
	s.buffer.Add(*price)
	return nil
}

func (s *Store) SaveBatch(_ context.Context, prices []model.Price) error {
	for _, p := range prices {
		s.buffer.Add(p)
	}
	return nil
}

func (s *Store) flushBatch(prices []model.Price) error {
	ctx := context.Background()
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO prices")
	if err != nil {
		return fmt.Errorf("preparing batch: %w", err)
	}

	for _, p := range prices {
		val, _ := p.Value.Float64()
		mc, _ := p.MarketCap.Float64()
		vol, _ := p.Volume24h.Float64()
		ts := p.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}
		ra := p.ReceivedAt
		if ra.IsZero() {
			ra = time.Now()
		}
		if err := batch.Append(
			p.CoinID, p.Currency,
			val,
			p.Provider,
			mc, vol, p.Change24h,
			ts, ra,
		); err != nil {
			return fmt.Errorf("appending to batch: %w", err)
		}
	}

	return batch.Send()
}

func (s *Store) GetLatest(ctx context.Context, coinID, currency string) (*model.Price, error) {
	var (
		cid, cur, provider   string
		val, mc, vol, change float64
		ts, ra               time.Time
	)
	err := s.conn.QueryRow(ctx,
		`SELECT coin_id, currency, value, provider, market_cap, volume_24h, change_24h, timestamp, received_at
         FROM prices
         WHERE coin_id = ? AND currency = ?
         ORDER BY received_at DESC
         LIMIT 1`, coinID, currency).Scan(&cid, &cur, &val, &provider, &mc, &vol, &change, &ts, &ra)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no rows") {
			return nil, nil
		}
		return nil, fmt.Errorf("querying latest: %w", err)
	}
	return &model.Price{
		CoinID: cid, Currency: cur,
		Value: decimal.NewFromFloat(val), Provider: provider,
		MarketCap: decimal.NewFromFloat(mc), Volume24h: decimal.NewFromFloat(vol),
		Change24h: change, Timestamp: ts, ReceivedAt: ra,
	}, nil
}

func (s *Store) GetHistory(ctx context.Context, coinID, currency string, from, to time.Time) ([]model.Price, error) {
	rows, err := s.conn.Query(ctx,
		`SELECT coin_id, currency, value, provider, market_cap, volume_24h, change_24h, timestamp, received_at
         FROM prices
         WHERE coin_id = ? AND currency = ?
           AND received_at >= ? AND received_at < ?
         ORDER BY received_at ASC`, coinID, currency, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var prices []model.Price
	for rows.Next() {
		var (
			cid, cur, provider   string
			val, mc, vol, change float64
			ts, ra               time.Time
		)
		if err := rows.Scan(&cid, &cur, &val, &provider, &mc, &vol, &change, &ts, &ra); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}
		prices = append(prices, model.Price{
			CoinID: cid, Currency: cur,
			Value: decimal.NewFromFloat(val), Provider: provider,
			MarketCap: decimal.NewFromFloat(mc), Volume24h: decimal.NewFromFloat(vol),
			Change24h: change, Timestamp: ts, ReceivedAt: ra,
		})
	}
	return prices, rows.Err()
}

func (s *Store) GetOHLC(ctx context.Context, coinID, currency string, from, to time.Time, interval time.Duration) ([]model.OHLC, error) {
	intervalSec := int(interval.Seconds())
	rows, err := s.conn.Query(ctx, fmt.Sprintf(
		`SELECT
            toStartOfInterval(received_at, INTERVAL %d second) AS time,
            argMin(value, received_at) AS open,
            max(value) AS high,
            min(value) AS low,
            argMax(value, received_at) AS close
         FROM prices
         WHERE coin_id = ? AND currency = ?
           AND received_at >= ? AND received_at < ?
         GROUP BY time
         ORDER BY time`, intervalSec), coinID, currency, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying ohlc: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []model.OHLC
	for rows.Next() {
		var (
			t                   time.Time
			open, high, low, cl float64
		)
		if err := rows.Scan(&t, &open, &high, &low, &cl); err != nil {
			return nil, fmt.Errorf("scanning ohlc: %w", err)
		}
		result = append(result, model.OHLC{
			Time:  t,
			Open:  decimal.NewFromFloat(open),
			High:  decimal.NewFromFloat(high),
			Low:   decimal.NewFromFloat(low),
			Close: decimal.NewFromFloat(cl),
		})
	}
	return result, rows.Err()
}
