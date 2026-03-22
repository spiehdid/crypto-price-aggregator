package router

import (
	"context"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
)

type ProviderRegistration struct {
	Provider    port.PriceProvider
	Tier        model.ProviderTier
	Priority    int
	RateLimit   int
	RateWindow  time.Duration
	CBThreshold int
	CBTimeout   time.Duration
	CostPerCall decimal.Decimal
}

type MultiRouter struct {
	mu        sync.Mutex
	strategy  Strategy
	providers map[string]*ProviderState
	ordered   []*ProviderState
}

func NewMultiRouter(strategy Strategy, registrations []ProviderRegistration) *MultiRouter {
	providers := make(map[string]*ProviderState, len(registrations))
	ordered := make([]*ProviderState, 0, len(registrations))
	for _, reg := range registrations {
		ps := &ProviderState{
			Provider:       reg.Provider,
			Tier:           reg.Tier,
			Priority:       reg.Priority,
			RateLimit:      NewRateLimitTracker(reg.RateLimit, reg.RateWindow),
			CircuitBreaker: NewCircuitBreaker(reg.CBThreshold, reg.CBTimeout),
			AvgLatency:     NewLatencyTracker(),
			CostPerCall:    reg.CostPerCall,
			Enabled:        true,
		}
		providers[reg.Provider.Name()] = ps
		ordered = append(ordered, ps)
	}
	return &MultiRouter{strategy: strategy, providers: providers, ordered: ordered}
}

// Select picks the best available provider and atomically consumes a rate-limit
// token so that concurrent callers cannot both claim the same slot.
func (r *MultiRouter) Select(_ context.Context, _ string) (port.PriceProvider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ps, err := r.strategy.Select(r.ordered)
	if err != nil {
		return nil, err
	}
	ps.RateLimit.Consume()
	return ps.Provider, nil
}

// ReportSuccess records a successful call. CircuitBreaker is self-synchronized;
// AvgLatency is protected by the per-provider mutex.
func (r *MultiRouter) ReportSuccess(provider string, latency time.Duration) {
	ps, ok := r.providers[provider]
	if !ok {
		return
	}
	ps.CircuitBreaker.RecordSuccess() // self-synchronized
	ps.mu.Lock()
	ps.AvgLatency.Record(float64(latency.Milliseconds()))
	ps.mu.Unlock()
}

// ReportFailure records a failed call. CircuitBreaker is self-synchronized.
func (r *MultiRouter) ReportFailure(provider string, err error) {
	ps, ok := r.providers[provider]
	if !ok {
		return
	}
	ps.CircuitBreaker.RecordFailure(err)
}

func (r *MultiRouter) ReportAccurate(provider string) {
	ps, ok := r.providers[provider]
	if !ok {
		return
	}
	ps.RecordAccurate()
}

func (r *MultiRouter) ReportOutlier(provider string) {
	ps, ok := r.providers[provider]
	if !ok {
		return
	}
	ps.RecordOutlier()
}

func (r *MultiRouter) ProviderStatuses() []model.ProviderStatus {
	var statuses []model.ProviderStatus
	for _, ps := range r.ordered {
		status := ps.Provider.Status()
		status.RemainingRate = ps.RateLimit.Remaining()
		ps.mu.Lock()
		avgLatency := ps.AvgLatency.Average()
		ps.mu.Unlock()
		status.AvgLatency = time.Duration(avgLatency) * time.Millisecond
		statuses = append(statuses, *status)
	}
	return statuses
}
