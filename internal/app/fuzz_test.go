package app_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

func FuzzOutlierDetector(f *testing.F) {
	f.Add(67000.0, 67500.0)
	f.Add(0.0, 67000.0)
	f.Add(67000.0, 0.0)
	f.Add(-1.0, 67000.0)
	f.Add(0.00000001, 0.00000002)

	d := app.NewOutlierDetector(5.0)

	f.Fuzz(func(t *testing.T, priceVal, consensusVal float64) {
		price := &model.Price{Value: decimal.NewFromFloat(priceVal)}
		consensus := decimal.NewFromFloat(consensusVal)
		// Should never panic
		_ = d.IsOutlier(price, consensus)
		_ = d.Deviation(price, consensus)
	})
}
