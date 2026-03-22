package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type Analytics struct {
	conn driver.Conn
}

func NewAnalytics(conn driver.Conn) *Analytics {
	return &Analytics{conn: conn}
}

func (a *Analytics) ProviderAccuracy(ctx context.Context, from, to time.Time) ([]model.ProviderAccuracyReport, error) {
	rows, err := a.conn.Query(ctx, `
        WITH medians AS (
            SELECT coin_id, currency,
                   toStartOfMinute(received_at) AS minute,
                   medianExact(value) AS median_val
            FROM prices
            WHERE received_at >= ? AND received_at < ?
            GROUP BY coin_id, currency, minute
        )
        SELECT
            p.provider,
            count() AS total,
            countIf(abs(p.value - m.median_val) / m.median_val < 0.02) AS accurate
        FROM prices p
        INNER JOIN medians m
            ON p.coin_id = m.coin_id
            AND p.currency = m.currency
            AND toStartOfMinute(p.received_at) = m.minute
        WHERE p.received_at >= ? AND p.received_at < ?
        GROUP BY p.provider
        ORDER BY accurate / total DESC`, from, to, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying provider accuracy: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []model.ProviderAccuracyReport
	for rows.Next() {
		var r model.ProviderAccuracyReport
		if err := rows.Scan(&r.Provider, &r.Total, &r.Accurate); err != nil {
			return nil, err
		}
		if r.Total > 0 {
			r.AccuracyRate = float64(r.Accurate) / float64(r.Total)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (a *Analytics) PriceAnomalies(ctx context.Context, coinID, currency string, deviationPercent float64, from, to time.Time) ([]model.PriceAnomaly, error) {
	rows, err := a.conn.Query(ctx, `
        WITH medians AS (
            SELECT toStartOfMinute(received_at) AS minute,
                   medianExact(value) AS median_val
            FROM prices
            WHERE coin_id = ? AND currency = ?
              AND received_at >= ? AND received_at < ?
            GROUP BY minute
        )
        SELECT
            p.received_at, p.provider, p.coin_id, p.currency,
            p.value, m.median_val,
            abs(p.value - m.median_val) / m.median_val * 100 AS deviation_pct
        FROM prices p
        INNER JOIN medians m ON toStartOfMinute(p.received_at) = m.minute
        WHERE p.coin_id = ? AND p.currency = ?
          AND p.received_at >= ? AND p.received_at < ?
        HAVING deviation_pct > ?
        ORDER BY p.received_at DESC
        LIMIT 1000`, coinID, currency, from, to, coinID, currency, from, to, deviationPercent)
	if err != nil {
		return nil, fmt.Errorf("querying anomalies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []model.PriceAnomaly
	for rows.Next() {
		var an model.PriceAnomaly
		var val, median float64
		if err := rows.Scan(&an.ReceivedAt, &an.Provider, &an.CoinID, &an.Currency, &val, &median, &an.DeviationPct); err != nil {
			return nil, err
		}
		an.Value = decimal.NewFromFloat(val)
		an.MedianValue = decimal.NewFromFloat(median)
		result = append(result, an)
	}
	return result, rows.Err()
}

func (a *Analytics) VolumeByProvider(ctx context.Context, from, to time.Time) ([]model.ProviderVolume, error) {
	rows, err := a.conn.Query(ctx, `
        SELECT provider, count() AS cnt
        FROM prices
        WHERE received_at >= ? AND received_at < ?
        GROUP BY provider
        ORDER BY cnt DESC`, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying volume: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []model.ProviderVolume
	var total int64
	for rows.Next() {
		var v model.ProviderVolume
		if err := rows.Scan(&v.Provider, &v.QueryCount); err != nil {
			return nil, err
		}
		total += v.QueryCount
		result = append(result, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range result {
		if total > 0 {
			result[i].Percentage = float64(result[i].QueryCount) / float64(total) * 100
		}
	}
	return result, nil
}

func (a *Analytics) PriceStats(ctx context.Context, coinID, currency string, from, to time.Time) (*model.PriceStatsSummary, error) {
	var min, max, avg, stddev float64
	var points uint64
	err := a.conn.QueryRow(ctx, `
        SELECT
            min(value), max(value), avg(value), stddevPop(value), count()
        FROM prices
        WHERE coin_id = ? AND currency = ?
          AND received_at >= ? AND received_at < ?`, coinID, currency, from, to).Scan(&min, &max, &avg, &stddev, &points)
	if err != nil {
		return nil, fmt.Errorf("querying price stats: %w", err)
	}
	return &model.PriceStatsSummary{
		CoinID: coinID, Currency: currency,
		Min: decimal.NewFromFloat(min), Max: decimal.NewFromFloat(max),
		Avg: decimal.NewFromFloat(avg), Stddev: stddev, Points: int64(points),
	}, nil
}
