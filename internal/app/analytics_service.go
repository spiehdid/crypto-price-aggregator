package app

import (
	"context"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/port"
)

type AnalyticsService struct {
	store port.AnalyticsStore
}

func NewAnalyticsService(store port.AnalyticsStore) *AnalyticsService {
	return &AnalyticsService{store: store}
}

func (s *AnalyticsService) GetProviderAccuracy(ctx context.Context, from, to time.Time) ([]model.ProviderAccuracyReport, error) {
	return s.store.ProviderAccuracy(ctx, from, to)
}

func (s *AnalyticsService) GetAnomalies(ctx context.Context, coinID, currency string, deviation float64, from, to time.Time) ([]model.PriceAnomaly, error) {
	return s.store.PriceAnomalies(ctx, coinID, currency, deviation, from, to)
}

func (s *AnalyticsService) GetVolumeByProvider(ctx context.Context, from, to time.Time) ([]model.ProviderVolume, error) {
	return s.store.VolumeByProvider(ctx, from, to)
}

func (s *AnalyticsService) GetPriceStats(ctx context.Context, coinID, currency string, from, to time.Time) (*model.PriceStatsSummary, error) {
	return s.store.PriceStats(ctx, coinID, currency, from, to)
}
