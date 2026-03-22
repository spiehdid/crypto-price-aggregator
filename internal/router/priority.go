package router

import (
	"sort"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type PriorityStrategy struct {
	allowPaid bool
}

func NewPriorityStrategy(allowPaid bool) *PriorityStrategy {
	return &PriorityStrategy{allowPaid: allowPaid}
}

func (s *PriorityStrategy) Select(providers []*ProviderState) (*ProviderState, error) {
	sorted := make([]*ProviderState, len(providers))
	copy(sorted, providers)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })
	for _, ps := range sorted {
		if ps.IsAvailable(s.allowPaid) {
			return ps, nil
		}
	}
	return nil, model.ErrNoHealthyProvider
}
