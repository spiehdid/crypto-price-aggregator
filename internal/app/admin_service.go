package app

import (
	"context"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/scheduler"
)

type ProviderStatusGetter interface {
	ProviderStatuses() []model.ProviderStatus
}

type AdminService struct {
	providers ProviderStatusGetter
	subSvc    *SubscriptionService
	priceSvc  *PriceService
	alertSvc  *AlertService
}

func NewAdminService(providers ProviderStatusGetter, subSvc *SubscriptionService, priceSvc *PriceService, alertSvc *AlertService) *AdminService {
	return &AdminService{
		providers: providers,
		subSvc:    subSvc,
		priceSvc:  priceSvc,
		alertSvc:  alertSvc,
	}
}

func (s *AdminService) GetProviderStatuses() []model.ProviderStatus {
	return s.providers.ProviderStatuses()
}

func (s *AdminService) GetSubscriptions() []scheduler.Subscription {
	return s.subSvc.Subscriptions()
}

func (s *AdminService) AddSubscription(ctx context.Context, coinID, currency string, interval time.Duration) error {
	return s.subSvc.Subscribe(ctx, coinID, currency, interval)
}

func (s *AdminService) RemoveSubscription(coinID, currency string) error {
	return s.subSvc.Unsubscribe(coinID, currency)
}

type Stats struct {
	TotalRequests       int64   `json:"total_requests"`
	CacheHits           int64   `json:"cache_hits"`
	CacheHitRate        float64 `json:"cache_hit_rate"`
	UptimeSeconds       int64   `json:"uptime_seconds"`
	HealthyProviders    int     `json:"healthy_providers"`
	TotalProviders      int     `json:"total_providers"`
	ActiveSubscriptions int     `json:"active_subscriptions"`
	ActiveAlerts        int     `json:"active_alerts"`
	TriggeredAlerts     int     `json:"triggered_alerts"`
}

func (s *AdminService) GetStats() Stats {
	var totalReqs, cacheHits int64
	var uptimeSecs int64
	if s.priceSvc != nil {
		var uptime time.Duration
		totalReqs, cacheHits, uptime = s.priceSvc.Stats()
		uptimeSecs = int64(uptime.Seconds())
	}

	var cacheHitRate float64
	if totalReqs > 0 {
		cacheHitRate = float64(cacheHits) / float64(totalReqs)
	}

	statuses := s.providers.ProviderStatuses()
	healthyCount := 0
	for _, ps := range statuses {
		if ps.Healthy {
			healthyCount++
		}
	}

	var subCount int
	if s.subSvc != nil {
		subCount = len(s.subSvc.Subscriptions())
	}

	activeAlerts := 0
	triggeredAlerts := 0
	if s.alertSvc != nil {
		for _, a := range s.alertSvc.List() {
			if a.Active {
				activeAlerts++
			} else if a.TriggeredAt != nil {
				triggeredAlerts++
			}
		}
	}

	return Stats{
		TotalRequests:       totalReqs,
		CacheHits:           cacheHits,
		CacheHitRate:        cacheHitRate,
		UptimeSeconds:       uptimeSecs,
		HealthyProviders:    healthyCount,
		TotalProviders:      len(statuses),
		ActiveSubscriptions: subCount,
		ActiveAlerts:        activeAlerts,
		TriggeredAlerts:     triggeredAlerts,
	}
}
