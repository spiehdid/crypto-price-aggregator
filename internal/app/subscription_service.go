package app

import (
	"context"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/spiehdid/crypto-price-aggregator/internal/scheduler"
)

type priceFetcherAdapter struct {
	svc *PriceService
}

func (a *priceFetcherAdapter) FetchPrice(ctx context.Context, coinID, currency string) (*model.Price, error) {
	return a.svc.GetPrice(ctx, coinID, currency)
}

type SubscriptionService struct {
	poller   *scheduler.Poller
	priceSvc *PriceService
}

func NewSubscriptionService(ctx context.Context, priceSvc *PriceService, minInterval time.Duration) *SubscriptionService {
	fetcher := &priceFetcherAdapter{svc: priceSvc}
	return &SubscriptionService{
		poller:   scheduler.NewPoller(ctx, fetcher, minInterval),
		priceSvc: priceSvc,
	}
}

// Start is a no-op kept for backward compatibility. The context is set in NewSubscriptionService.
func (s *SubscriptionService) Start(_ context.Context) {}

func (s *SubscriptionService) Stop() {
	s.poller.Stop()
}

func (s *SubscriptionService) Subscribe(ctx context.Context, coinID, currency string, interval time.Duration) error {
	return s.poller.Subscribe(scheduler.Subscription{
		CoinID: coinID, Currency: currency, Interval: interval,
	})
}

func (s *SubscriptionService) Unsubscribe(coinID, currency string) error {
	return s.poller.Unsubscribe(coinID, currency)
}

func (s *SubscriptionService) Subscriptions() []scheduler.Subscription {
	return s.poller.Subscriptions()
}
