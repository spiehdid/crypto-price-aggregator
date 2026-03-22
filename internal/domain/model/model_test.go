package model_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
)

// --- AlertCondition.String() ---

func TestAlertCondition_StringAbove(t *testing.T) {
	assert.Equal(t, "above", model.ConditionAbove.String())
}

func TestAlertCondition_StringBelow(t *testing.T) {
	assert.Equal(t, "below", model.ConditionBelow.String())
}

func TestAlertCondition_StringUnknown(t *testing.T) {
	unknown := model.AlertCondition(99)
	assert.Equal(t, "unknown", unknown.String())
}

// --- Alert struct fields ---

func TestAlert_Fields(t *testing.T) {
	now := time.Now()
	triggered := now.Add(time.Minute)
	a := model.Alert{
		ID:          "alert-1",
		CoinID:      "bitcoin",
		Currency:    "usd",
		Condition:   model.ConditionAbove,
		Threshold:   decimal.NewFromFloat(70000),
		WebhookURL:  "https://example.com/hook",
		Active:      true,
		CreatedAt:   now,
		TriggeredAt: &triggered,
	}
	assert.Equal(t, "alert-1", a.ID)
	assert.Equal(t, "bitcoin", a.CoinID)
	assert.Equal(t, "usd", a.Currency)
	assert.Equal(t, model.ConditionAbove, a.Condition)
	assert.True(t, a.Threshold.Equal(decimal.NewFromFloat(70000)))
	assert.Equal(t, "https://example.com/hook", a.WebhookURL)
	assert.True(t, a.Active)
	assert.Equal(t, now, a.CreatedAt)
	assert.Equal(t, triggered, *a.TriggeredAt)
}

// --- Price with MarketCap, Volume24h, Change24h ---

func TestPrice_ExtendedFields(t *testing.T) {
	p := model.Price{
		CoinID:    "bitcoin",
		Currency:  "usd",
		Value:     decimal.NewFromFloat(67000),
		Provider:  "coingecko",
		MarketCap: decimal.NewFromFloat(1_300_000_000_000),
		Volume24h: decimal.NewFromFloat(25_000_000_000),
		Change24h: 2.75,
	}

	assert.Equal(t, "bitcoin", p.CoinID)
	assert.True(t, p.MarketCap.Equal(decimal.NewFromFloat(1_300_000_000_000)))
	assert.True(t, p.Volume24h.Equal(decimal.NewFromFloat(25_000_000_000)))
	assert.InDelta(t, 2.75, p.Change24h, 0.001)
}

func TestPrice_NegativeChange24h(t *testing.T) {
	p := model.Price{
		CoinID:    "ethereum",
		Currency:  "usd",
		Value:     decimal.NewFromFloat(3000),
		Change24h: -5.12,
	}
	assert.InDelta(t, -5.12, p.Change24h, 0.001)
}

// --- ProviderTier constants ---

func TestProviderTier_Values(t *testing.T) {
	assert.Equal(t, model.ProviderTier(0), model.TierFree)
	assert.Equal(t, model.ProviderTier(1), model.TierPaid)
	assert.NotEqual(t, model.TierFree, model.TierPaid)
}

func TestProviderStatus_TierField(t *testing.T) {
	ps := model.ProviderStatus{
		Name:    "coingecko",
		Healthy: true,
		Tier:    model.TierFree,
	}
	assert.Equal(t, model.TierFree, ps.Tier)

	ps2 := model.ProviderStatus{
		Name:    "coinmarketcap",
		Healthy: true,
		Tier:    model.TierPaid,
	}
	assert.Equal(t, model.TierPaid, ps2.Tier)
}

// --- Sentinel errors are distinct and non-nil ---

func TestErrors_NotNil(t *testing.T) {
	errs := []error{
		model.ErrRateLimited,
		model.ErrProviderDown,
		model.ErrCoinNotFound,
		model.ErrUnauthorized,
		model.ErrNoHealthyProvider,
		model.ErrBudgetExhausted,
		model.ErrStoreUnavailable,
	}
	for _, err := range errs {
		assert.NotNil(t, err)
	}
}

func TestErrors_Distinct(t *testing.T) {
	assert.NotEqual(t, model.ErrRateLimited, model.ErrProviderDown)
	assert.NotEqual(t, model.ErrCoinNotFound, model.ErrUnauthorized)
	assert.NotEqual(t, model.ErrNoHealthyProvider, model.ErrBudgetExhausted)
}

// --- Coin struct ---

func TestCoin_Aliases(t *testing.T) {
	c := model.Coin{
		ID:     "bitcoin",
		Symbol: "btc",
		Name:   "Bitcoin",
		Aliases: map[string]string{
			"coingecko": "bitcoin",
			"binance":   "BTCUSDT",
		},
	}
	assert.Equal(t, "bitcoin", c.Aliases["coingecko"])
	assert.Equal(t, "BTCUSDT", c.Aliases["binance"])
}
