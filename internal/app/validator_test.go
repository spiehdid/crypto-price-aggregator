package app_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
)

func TestPriceValidator_ValidPrice(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{Enabled: true, MaxStaleness: 5 * time.Minute})
	price := &model.Price{Value: decimal.NewFromFloat(67000), Timestamp: time.Now()}
	assert.NoError(t, v.Validate(price))
}

func TestPriceValidator_ZeroPrice(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{Enabled: true})
	price := &model.Price{Value: decimal.Zero}
	assert.ErrorIs(t, v.Validate(price), app.ErrPriceZero)
}

func TestPriceValidator_NegativePrice(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{Enabled: true})
	price := &model.Price{Value: decimal.NewFromFloat(-100)}
	assert.ErrorIs(t, v.Validate(price), app.ErrPriceZero)
}

func TestPriceValidator_StalePrice(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{Enabled: true, MaxStaleness: 1 * time.Minute})
	price := &model.Price{Value: decimal.NewFromFloat(67000), Timestamp: time.Now().Add(-2 * time.Minute)}
	assert.ErrorIs(t, v.Validate(price), app.ErrPriceStale)
}

func TestPriceValidator_Disabled(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{Enabled: false})
	price := &model.Price{Value: decimal.Zero}
	assert.NoError(t, v.Validate(price))
}

func TestPriceValidator_DeviationFromReference(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{Enabled: true, MaxDeviationFromCache: 5.0})
	price := &model.Price{Value: decimal.NewFromFloat(70000)}
	reference := &model.Price{Value: decimal.NewFromFloat(67000)}

	// ~4.5% deviation — within 5% threshold
	assert.NoError(t, v.ValidateAgainstReference(price, reference))
}

func TestPriceValidator_ExceedsDeviation(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{Enabled: true, MaxDeviationFromCache: 5.0})
	price := &model.Price{Value: decimal.NewFromFloat(80000)}
	reference := &model.Price{Value: decimal.NewFromFloat(67000)}

	// ~19.4% deviation — exceeds 5%
	assert.ErrorIs(t, v.ValidateAgainstReference(price, reference), app.ErrInvalidPrice)
}

func TestPriceValidator_NilReference(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{Enabled: true, MaxDeviationFromCache: 5.0})
	price := &model.Price{Value: decimal.NewFromFloat(70000)}
	assert.NoError(t, v.ValidateAgainstReference(price, nil))
}

func TestPriceValidator_StablecoinDeviation(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{
		Enabled:               true,
		MaxDeviationFromCache: 5.0,
		StablecoinDeviation:   2.0,
	})

	// USDT: 1% deviation — within 2% stablecoin threshold
	price := &model.Price{CoinID: "tether", Value: decimal.NewFromFloat(1.01)}
	ref := &model.Price{CoinID: "tether", Value: decimal.NewFromFloat(1.00)}
	assert.NoError(t, v.ValidateAgainstReference(price, ref))

	// USDT: 3% deviation — exceeds 2% stablecoin threshold
	price2 := &model.Price{CoinID: "tether", Value: decimal.NewFromFloat(1.03)}
	assert.ErrorIs(t, v.ValidateAgainstReference(price2, ref), app.ErrInvalidPrice)
}

func TestPriceValidator_MajorCoinDeviation(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{
		Enabled:               true,
		MaxDeviationFromCache: 5.0,
		MajorCoinDeviation:    10.0,
	})

	// BTC: 8% deviation — within 10% major coin threshold (would exceed default 5%)
	price := &model.Price{CoinID: "bitcoin", Value: decimal.NewFromFloat(72000)}
	ref := &model.Price{CoinID: "bitcoin", Value: decimal.NewFromFloat(67000)}
	assert.NoError(t, v.ValidateAgainstReference(price, ref))

	// BTC: 15% deviation — exceeds 10% major coin threshold
	price2 := &model.Price{CoinID: "bitcoin", Value: decimal.NewFromFloat(77000)}
	assert.ErrorIs(t, v.ValidateAgainstReference(price2, ref), app.ErrInvalidPrice)
}

func TestPriceValidator_AltcoinDeviation(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{
		Enabled:               true,
		MaxDeviationFromCache: 5.0,
		AltcoinDeviation:      20.0,
	})

	// Unknown altcoin: 15% deviation — within 20% altcoin threshold (would exceed default 5%)
	price := &model.Price{CoinID: "some-altcoin", Value: decimal.NewFromFloat(115)}
	ref := &model.Price{CoinID: "some-altcoin", Value: decimal.NewFromFloat(100)}
	assert.NoError(t, v.ValidateAgainstReference(price, ref))

	// Unknown altcoin: 25% deviation — exceeds 20%
	price2 := &model.Price{CoinID: "some-altcoin", Value: decimal.NewFromFloat(125)}
	assert.ErrorIs(t, v.ValidateAgainstReference(price2, ref), app.ErrInvalidPrice)
}

func TestPriceValidator_StaleReference(t *testing.T) {
	v := app.NewPriceValidator(app.PriceValidatorConfig{
		Enabled:               true,
		MaxStaleness:          5 * time.Minute,
		MaxDeviationFromCache: 5.0,
	})

	// Reference is 15 minutes old (> 2 * 5m = 10m) — skip comparison
	staleRef := &model.Price{
		Value:      decimal.NewFromFloat(67000),
		ReceivedAt: time.Now().Add(-15 * time.Minute),
	}
	price := &model.Price{CoinID: "bitcoin", Value: decimal.NewFromFloat(80000)}
	// Would normally exceed 5%, but stale reference means skip
	assert.NoError(t, v.ValidateAgainstReference(price, staleRef))
}
