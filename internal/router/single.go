package router

import (
	"context"
	"sync"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
)

const defaultFailureThreshold = 5

type SingleRouter struct {
	provider         port.PriceProvider
	mu               sync.RWMutex
	failCount        int
	failureThreshold int
}

func NewSingleRouter(provider port.PriceProvider) *SingleRouter {
	return &SingleRouter{
		provider:         provider,
		failureThreshold: defaultFailureThreshold,
	}
}

func (r *SingleRouter) Select(_ context.Context, _ string) (port.PriceProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.failCount >= r.failureThreshold {
		return nil, model.ErrNoHealthyProvider
	}

	return r.provider, nil
}

func (r *SingleRouter) ReportSuccess(_ string, _ time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failCount = 0
}

func (r *SingleRouter) ReportFailure(_ string, _ error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failCount++
}
