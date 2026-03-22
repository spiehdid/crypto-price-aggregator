package model

import "time"

type ProviderTier int

const (
	TierFree ProviderTier = iota
	TierPaid
)

type ProviderStatus struct {
	Name          string
	Healthy       bool
	RemainingRate int
	ResetAt       time.Time
	AvgLatency    time.Duration
	Tier          ProviderTier
	LastError     error
	LastSuccessAt time.Time
}
