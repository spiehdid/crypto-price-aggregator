package router_test

import (
	"testing"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/router"
	"github.com/stretchr/testify/assert"
)

func TestRateLimitTracker_InitialState(t *testing.T) {
	rl := router.NewRateLimitTracker(30, time.Minute)
	assert.Equal(t, 30, rl.Remaining())
	assert.False(t, rl.IsExhausted())
}

func TestRateLimitTracker_Consume(t *testing.T) {
	rl := router.NewRateLimitTracker(3, time.Minute)
	rl.Consume()
	assert.Equal(t, 2, rl.Remaining())
	rl.Consume()
	rl.Consume()
	assert.Equal(t, 0, rl.Remaining())
	assert.True(t, rl.IsExhausted())
}

func TestRateLimitTracker_ConsumeNeverNegative(t *testing.T) {
	rl := router.NewRateLimitTracker(1, time.Minute)
	rl.Consume()
	rl.Consume()
	assert.Equal(t, 0, rl.Remaining())
}

func TestRateLimitTracker_Reset(t *testing.T) {
	rl := router.NewRateLimitTracker(3, 50*time.Millisecond)
	rl.Consume()
	rl.Consume()
	rl.Consume()
	assert.True(t, rl.IsExhausted())
	time.Sleep(60 * time.Millisecond)
	assert.False(t, rl.IsExhausted())
	assert.Equal(t, 3, rl.Remaining())
}

func TestRateLimitTracker_RemainingRatio(t *testing.T) {
	rl := router.NewRateLimitTracker(10, time.Minute)
	assert.Equal(t, 1.0, rl.RemainingRatio())
	rl.Consume()
	rl.Consume()
	assert.InDelta(t, 0.8, rl.RemainingRatio(), 0.01)
}
