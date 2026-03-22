package model

import (
	"time"

	"github.com/shopspring/decimal"
)

type Price struct {
	CoinID     string
	Currency   string
	Value      decimal.Decimal
	Provider   string
	Timestamp  time.Time
	ReceivedAt time.Time
	MarketCap  decimal.Decimal
	Volume24h  decimal.Decimal
	// Change24h is a percentage (e.g., 2.5 means +2.5%). Stored as float64
	// because it is a derived metric (not an absolute price) and does not
	// require decimal precision. Acceptable range: -100 to +∞.
	Change24h float64
}

func (p Price) IsStale(ttl time.Duration) bool {
	return time.Since(p.ReceivedAt) > ttl
}

func (p Price) Age() time.Duration {
	return time.Since(p.ReceivedAt)
}

type Coin struct {
	ID      string
	Symbol  string
	Name    string
	Aliases map[string]string // provider name → provider-specific coin ID
}
