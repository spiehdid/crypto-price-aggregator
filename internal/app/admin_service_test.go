package app_test

import (
	"testing"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
)

type mockProviderStatusGetter struct{}

func (m *mockProviderStatusGetter) ProviderStatuses() []model.ProviderStatus {
	return []model.ProviderStatus{
		{Name: "coingecko", Healthy: true, Tier: model.TierFree, RemainingRate: 25, AvgLatency: 100 * time.Millisecond},
		{Name: "coinmarketcap", Healthy: true, Tier: model.TierFree, RemainingRate: 28, AvgLatency: 150 * time.Millisecond},
	}
}

func TestAdminService_GetProviderStatuses(t *testing.T) {
	adminSvc := app.NewAdminService(&mockProviderStatusGetter{}, nil, nil, nil)
	statuses := adminSvc.GetProviderStatuses()
	assert.Len(t, statuses, 2)
	assert.Equal(t, "coingecko", statuses[0].Name)
}
