package router

import (
	"sync"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
)

type Strategy interface {
	Select(providers []*ProviderState) (*ProviderState, error)
}

type ProviderState struct {
	mu             sync.Mutex
	Provider       port.PriceProvider
	Tier           model.ProviderTier
	Priority       int
	RateLimit      *RateLimitTracker
	CircuitBreaker *CircuitBreaker
	AvgLatency     *LatencyTracker
	CostPerCall    decimal.Decimal
	Enabled        bool
	AccuracyScore  float64 // 0.0-1.0, rolling accuracy
	OutlierCount   int     // consecutive outlier responses
	TotalResponses int64
	AccurateCount  int64
}

func (ps *ProviderState) IsAvailable(allowPaid bool) bool {
	if !ps.Enabled {
		return false
	}
	if !allowPaid && ps.Tier == model.TierPaid {
		return false
	}
	if !ps.CircuitBreaker.AllowRequest() {
		return false
	}
	if ps.RateLimit.IsExhausted() {
		return false
	}
	ps.mu.Lock()
	outlierCount := ps.OutlierCount
	ps.mu.Unlock()
	return outlierCount < 5
}

func (ps *ProviderState) RecordAccurate() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.TotalResponses++
	ps.AccurateCount++
	ps.OutlierCount = 0
	ps.AccuracyScore = float64(ps.AccurateCount) / float64(ps.TotalResponses)
}

func (ps *ProviderState) RecordOutlier() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.TotalResponses++
	ps.OutlierCount++
	ps.AccuracyScore = float64(ps.AccurateCount) / float64(ps.TotalResponses)
}

type ProviderSnapshot struct {
	AccuracyScore  float64
	TotalResponses int64
	AvgLatencyMs   float64
	RemainingRatio float64
	CostPerCall    float64
}

func (ps *ProviderState) Snapshot() ProviderSnapshot {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ProviderSnapshot{
		AccuracyScore:  ps.AccuracyScore,
		TotalResponses: ps.TotalResponses,
		AvgLatencyMs:   ps.AvgLatency.averageUnsafe(),
		RemainingRatio: ps.RateLimit.RemainingRatio(),
		CostPerCall:    ps.CostPerCall.InexactFloat64(),
	}
}

type LatencyTracker struct {
	avg   float64
	alpha float64
	count int
}

func NewLatencyTracker() *LatencyTracker {
	return &LatencyTracker{alpha: 0.3}
}

// Record updates the exponential moving average. Must be called while holding
// the enclosing ProviderState.mu lock.
func (lt *LatencyTracker) Record(ms float64) {
	if lt.count == 0 {
		lt.avg = ms
	} else {
		lt.avg = lt.alpha*ms + (1-lt.alpha)*lt.avg
	}
	lt.count++
}

// Average returns the current average. Must be called while holding the
// enclosing ProviderState.mu lock, or use averageUnsafe from Snapshot.
func (lt *LatencyTracker) Average() float64 {
	return lt.avg
}

// averageUnsafe reads avg without any locking. Only safe to call when the
// caller already holds the ProviderState.mu lock (e.g. from Snapshot).
func (lt *LatencyTracker) averageUnsafe() float64 {
	return lt.avg
}
