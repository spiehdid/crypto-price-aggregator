package app_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
)

func TestConsensusEngine_ComputeResult_Median(t *testing.T) {
	engine := app.NewConsensusEngine(app.ConsensusConfig{
		Enabled:      true,
		MinProviders: 3,
		MaxDeviation: 5.0,
		Timeout:      5 * time.Second,
	})

	prices := []model.Price{
		{CoinID: "bitcoin", Value: decimal.NewFromFloat(67000), Provider: "a"},
		{CoinID: "bitcoin", Value: decimal.NewFromFloat(67100), Provider: "b"},
		{CoinID: "bitcoin", Value: decimal.NewFromFloat(67050), Provider: "c"},
		{CoinID: "bitcoin", Value: decimal.NewFromFloat(67200), Provider: "d"},
		{CoinID: "bitcoin", Value: decimal.NewFromFloat(5000), Provider: "bad"},
	}

	result := engine.ComputeResultPublic(prices)

	// Median of sorted [5000, 67000, 67050, 67100, 67200] = 67050
	assert.True(t, decimal.NewFromFloat(67050).Equal(result.MedianPrice))
	assert.Len(t, result.Outliers, 1) // 5000 is an outlier
	assert.Equal(t, "bad", result.Outliers[0].Provider)
	assert.Greater(t, result.Confidence, 0.7)
}

func TestConsensusEngine_EvenNumberOfPrices(t *testing.T) {
	engine := app.NewConsensusEngine(app.ConsensusConfig{
		Enabled: true, MinProviders: 2, MaxDeviation: 5.0, Timeout: 5 * time.Second,
	})

	prices := []model.Price{
		{Value: decimal.NewFromFloat(67000), Provider: "a"},
		{Value: decimal.NewFromFloat(67100), Provider: "b"},
	}

	result := engine.ComputeResultPublic(prices)
	expected := decimal.NewFromFloat(67050) // average of middle two
	assert.True(t, expected.Equal(result.MedianPrice))
}

func TestConsensusEngine_AllOutliers(t *testing.T) {
	engine := app.NewConsensusEngine(app.ConsensusConfig{
		Enabled: true, MinProviders: 2, MaxDeviation: 1.0, Timeout: 5 * time.Second,
	})

	prices := []model.Price{
		{Value: decimal.NewFromFloat(67000), Provider: "a"},
		{Value: decimal.NewFromFloat(80000), Provider: "b"},
	}

	result := engine.ComputeResultPublic(prices)
	// Median = 73500, both deviate significantly
	assert.Greater(t, len(result.Outliers), 0)
}
