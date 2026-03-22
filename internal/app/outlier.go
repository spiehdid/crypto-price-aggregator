package app

import (
	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type OutlierDetector struct {
	maxDeviationPercent float64
}

func NewOutlierDetector(maxDeviationPercent float64) *OutlierDetector {
	return &OutlierDetector{maxDeviationPercent: maxDeviationPercent}
}

func (d *OutlierDetector) IsOutlier(price *model.Price, consensus decimal.Decimal) bool {
	if consensus.IsZero() {
		return false
	}

	deviation := price.Value.Sub(consensus).Abs().Div(consensus).
		Mul(decimal.NewFromInt(100))

	return deviation.GreaterThan(decimal.NewFromFloat(d.maxDeviationPercent))
}

func (d *OutlierDetector) Deviation(price *model.Price, consensus decimal.Decimal) float64 {
	if consensus.IsZero() {
		return 0
	}

	deviation := price.Value.Sub(consensus).Abs().Div(consensus).
		Mul(decimal.NewFromInt(100))

	f, _ := deviation.Float64()
	return f
}
