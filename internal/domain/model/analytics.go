package model

import (
	"time"

	"github.com/shopspring/decimal"
)

type ProviderAccuracyReport struct {
	Provider     string  `json:"provider"`
	Total        int64   `json:"total"`
	Accurate     int64   `json:"accurate"`
	AccuracyRate float64 `json:"accuracy_rate"`
}

type PriceAnomaly struct {
	ReceivedAt   time.Time       `json:"received_at"`
	Provider     string          `json:"provider"`
	CoinID       string          `json:"coin_id"`
	Currency     string          `json:"currency"`
	Value        decimal.Decimal `json:"value"`
	MedianValue  decimal.Decimal `json:"median_value"`
	DeviationPct float64         `json:"deviation_pct"`
}

type ProviderVolume struct {
	Provider   string  `json:"provider"`
	QueryCount int64   `json:"query_count"`
	Percentage float64 `json:"percentage"`
}

type PriceStatsSummary struct {
	CoinID   string          `json:"coin_id"`
	Currency string          `json:"currency"`
	Min      decimal.Decimal `json:"min"`
	Max      decimal.Decimal `json:"max"`
	Avg      decimal.Decimal `json:"avg"`
	Stddev   float64         `json:"stddev"`
	Points   int64           `json:"points"`
}
