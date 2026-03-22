package app

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

var (
	ErrInvalidPrice = fmt.Errorf("invalid price")
	ErrPriceTooLow  = fmt.Errorf("price below minimum threshold")
	ErrPriceTooHigh = fmt.Errorf("price above maximum threshold")
	ErrPriceStale   = fmt.Errorf("price is stale")
	ErrPriceZero    = fmt.Errorf("price is zero or negative")
)

type PriceValidatorConfig struct {
	Enabled               bool
	MaxStaleness          time.Duration
	MaxDeviationFromCache float64 // default for unknown coins
	StablecoinDeviation   float64 // e.g., 2% for USDT, USDC, DAI
	MajorCoinDeviation    float64 // e.g., 10% for BTC, ETH
	AltcoinDeviation      float64 // e.g., 20% for others
}

var stablecoins = map[string]bool{
	"tether": true, "usd-coin": true, "dai": true, "binance-usd": true,
	"trueusd": true, "frax": true, "usdd": true,
}

var majorCoins = map[string]bool{
	"bitcoin": true, "ethereum": true, "binancecoin": true, "solana": true,
	"cardano": true, "ripple": true, "polkadot": true, "avalanche-2": true,
}

type PriceValidator struct {
	cfg PriceValidatorConfig
}

func NewPriceValidator(cfg PriceValidatorConfig) *PriceValidator {
	return &PriceValidator{cfg: cfg}
}

func (v *PriceValidator) maxDeviationForCoin(coinID string) float64 {
	if v.cfg.StablecoinDeviation > 0 && stablecoins[coinID] {
		return v.cfg.StablecoinDeviation
	}
	if v.cfg.MajorCoinDeviation > 0 && majorCoins[coinID] {
		return v.cfg.MajorCoinDeviation
	}
	if v.cfg.AltcoinDeviation > 0 {
		return v.cfg.AltcoinDeviation
	}
	return v.cfg.MaxDeviationFromCache
}

func (v *PriceValidator) Validate(price *model.Price) error {
	if !v.cfg.Enabled {
		return nil
	}

	if price.Value.IsZero() || price.Value.IsNegative() {
		return ErrPriceZero
	}

	if v.cfg.MaxStaleness > 0 && !price.Timestamp.IsZero() {
		if time.Since(price.Timestamp) > v.cfg.MaxStaleness {
			return ErrPriceStale
		}
	}

	return nil
}

func (v *PriceValidator) ValidateAgainstReference(price *model.Price, reference *model.Price) error {
	if !v.cfg.Enabled || reference == nil || v.cfg.MaxDeviationFromCache <= 0 {
		return nil
	}

	if reference.Value.IsZero() {
		return nil
	}

	// Skip comparison if reference is too old (stale reference is unreliable)
	if v.cfg.MaxStaleness > 0 && !reference.ReceivedAt.IsZero() {
		if time.Since(reference.ReceivedAt) > v.cfg.MaxStaleness*2 {
			// Reference is more than 2x max staleness — too old to compare against
			return nil
		}
	}

	deviation := price.Value.Sub(reference.Value).Abs().Div(reference.Value).
		Mul(decimal.NewFromInt(100))

	maxDevFloat := v.maxDeviationForCoin(price.CoinID)
	maxDev := decimal.NewFromFloat(maxDevFloat)
	if deviation.GreaterThan(maxDev) {
		return fmt.Errorf("%w: deviation %.2f%% exceeds max %.2f%%",
			ErrInvalidPrice, deviation.InexactFloat64(), maxDevFloat)
	}

	return nil
}
