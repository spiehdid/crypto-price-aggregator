package router

import (
	"sync"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type RoundRobinStrategy struct {
	mu        sync.Mutex
	index     int
	allowPaid bool
}

func NewRoundRobinStrategy(allowPaid bool) *RoundRobinStrategy {
	return &RoundRobinStrategy{allowPaid: allowPaid}
}

func (s *RoundRobinStrategy) Select(providers []*ProviderState) (*ProviderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(providers)
	if n == 0 {
		return nil, model.ErrNoHealthyProvider
	}
	for i := 0; i < n; i++ {
		idx := (s.index + i) % n
		ps := providers[idx]
		if ps.IsAvailable(s.allowPaid) {
			s.index = (idx + 1) % n
			return ps, nil
		}
	}
	return nil, model.ErrNoHealthyProvider
}
