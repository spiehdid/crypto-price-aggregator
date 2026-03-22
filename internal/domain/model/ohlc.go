package model

import (
	"time"

	"github.com/shopspring/decimal"
)

type OHLC struct {
	Time  time.Time
	Open  decimal.Decimal
	High  decimal.Decimal
	Low   decimal.Decimal
	Close decimal.Decimal
}
