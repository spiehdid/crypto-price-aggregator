package model_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

func TestPrice_IsStale(t *testing.T) {
	now := time.Now()

	fresh := model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(67432.15),
		Provider:   "coingecko",
		Timestamp:  now,
		ReceivedAt: now,
	}

	stale := model.Price{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Value:      decimal.NewFromFloat(67432.15),
		Provider:   "coingecko",
		Timestamp:  now.Add(-2 * time.Minute),
		ReceivedAt: now.Add(-2 * time.Minute),
	}

	if fresh.IsStale(30 * time.Second) {
		t.Error("expected fresh price to not be stale")
	}

	if !stale.IsStale(30 * time.Second) {
		t.Error("expected stale price to be stale")
	}
}

func TestPrice_Age(t *testing.T) {
	p := model.Price{
		ReceivedAt: time.Now().Add(-45 * time.Second),
	}
	age := p.Age()
	if age < 44*time.Second || age > 46*time.Second {
		t.Errorf("expected age ~45s, got %v", age)
	}
}
