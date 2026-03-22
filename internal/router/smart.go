package router

import (
	"math"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type SmartWeights struct {
	RateRemaining float64
	Latency       float64
	Cost          float64
}

type SmartStrategy struct {
	weights   SmartWeights
	allowPaid bool
}

func NewSmartStrategy(weights SmartWeights, allowPaid bool) *SmartStrategy {
	return &SmartStrategy{weights: weights, allowPaid: allowPaid}
}

func (s *SmartStrategy) Select(providers []*ProviderState) (*ProviderState, error) {
	var best *ProviderState
	bestScore := math.Inf(-1)
	for _, ps := range providers {
		if !ps.IsAvailable(s.allowPaid) {
			continue
		}
		snap := ps.Snapshot()
		score := s.score(snap)
		if score > bestScore {
			bestScore = score
			best = ps
		}
	}
	if best == nil {
		return nil, model.ErrNoHealthyProvider
	}
	return best, nil
}

func (s *SmartStrategy) score(snap ProviderSnapshot) float64 {
	rateScore := snap.RemainingRatio

	var latencyScore float64
	if snap.AvgLatencyMs > 0 {
		latencyScore = 1.0 / snap.AvgLatencyMs
	} else {
		latencyScore = 1.0
	}

	costScore := -snap.CostPerCall

	baseScore := s.weights.RateRemaining*rateScore +
		s.weights.Latency*latencyScore +
		s.weights.Cost*costScore

	accuracy := snap.AccuracyScore
	if snap.TotalResponses == 0 {
		accuracy = 1.0
	}

	return baseScore * accuracy
}
