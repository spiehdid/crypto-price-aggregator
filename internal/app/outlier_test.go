package app_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
)

func TestOutlierDetector_NotOutlier(t *testing.T) {
	d := app.NewOutlierDetector(5.0)
	price := &model.Price{Value: decimal.NewFromFloat(67500)}
	consensus := decimal.NewFromFloat(67000)
	assert.False(t, d.IsOutlier(price, consensus)) // ~0.75% deviation
}

func TestOutlierDetector_IsOutlier(t *testing.T) {
	d := app.NewOutlierDetector(5.0)
	price := &model.Price{Value: decimal.NewFromFloat(80000)}
	consensus := decimal.NewFromFloat(67000)
	assert.True(t, d.IsOutlier(price, consensus)) // ~19.4% deviation
}

func TestOutlierDetector_ZeroConsensus(t *testing.T) {
	d := app.NewOutlierDetector(5.0)
	price := &model.Price{Value: decimal.NewFromFloat(67000)}
	assert.False(t, d.IsOutlier(price, decimal.Zero))
}

func TestOutlierDetector_Deviation(t *testing.T) {
	d := app.NewOutlierDetector(5.0)
	price := &model.Price{Value: decimal.NewFromFloat(70000)}
	consensus := decimal.NewFromFloat(67000)
	dev := d.Deviation(price, consensus)
	assert.InDelta(t, 4.48, dev, 0.1)
}

func TestOutlierDetector_ExactMatch(t *testing.T) {
	d := app.NewOutlierDetector(5.0)
	price := &model.Price{Value: decimal.NewFromFloat(67000)}
	consensus := decimal.NewFromFloat(67000)
	assert.False(t, d.IsOutlier(price, consensus))
	assert.Equal(t, 0.0, d.Deviation(price, consensus))
}
