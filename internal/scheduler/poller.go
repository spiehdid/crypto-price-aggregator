package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type PriceFetcher interface {
	FetchPrice(ctx context.Context, coinID, currency string) (*model.Price, error)
}

type activeSub struct {
	sub    Subscription
	cancel context.CancelFunc
}

type Poller struct {
	fetcher     PriceFetcher
	minInterval time.Duration
	mu          sync.RWMutex
	subs        map[string]*activeSub
	ctx         context.Context
}

func NewPoller(ctx context.Context, fetcher PriceFetcher, minInterval time.Duration) *Poller {
	return &Poller{
		fetcher:     fetcher,
		minInterval: minInterval,
		subs:        make(map[string]*activeSub),
		ctx:         ctx,
	}
}

// Start is a no-op kept for backward compatibility. The context is set in NewPoller.
func (p *Poller) Start(_ context.Context) {}

func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, as := range p.subs {
		as.cancel()
		delete(p.subs, key)
	}
}

func (p *Poller) Subscribe(sub Subscription) error {
	if sub.Interval < p.minInterval {
		sub.Interval = p.minInterval
	}
	key := subKey(sub.CoinID, sub.Currency)
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.subs[key]; ok {
		existing.cancel()
	}
	subCtx, cancel := context.WithCancel(p.ctx)
	p.subs[key] = &activeSub{sub: sub, cancel: cancel}
	go p.poll(subCtx, sub)
	slog.Info("subscription started", "coin", sub.CoinID, "currency", sub.Currency, "interval", sub.Interval)
	return nil
}

func (p *Poller) Unsubscribe(coinID, currency string) error {
	key := subKey(coinID, currency)
	p.mu.Lock()
	defer p.mu.Unlock()
	as, ok := p.subs[key]
	if !ok {
		return fmt.Errorf("subscription not found: %s:%s", coinID, currency)
	}
	as.cancel()
	delete(p.subs, key)
	slog.Info("subscription stopped", "coin", coinID, "currency", currency)
	return nil
}

func (p *Poller) Subscriptions() []Subscription {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]Subscription, 0, len(p.subs))
	for _, as := range p.subs {
		result = append(result, as.sub)
	}
	return result
}

func (p *Poller) poll(ctx context.Context, sub Subscription) {
	ticker := time.NewTicker(sub.Interval)
	defer ticker.Stop()
	p.fetchOnce(ctx, sub)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.fetchOnce(ctx, sub)
		}
	}
}

func (p *Poller) fetchOnce(ctx context.Context, sub Subscription) {
	_, err := p.fetcher.FetchPrice(ctx, sub.CoinID, sub.Currency)
	if err != nil {
		slog.Warn("poll fetch failed", "coin", sub.CoinID, "currency", sub.Currency, "error", err)
	}
}

func subKey(coinID, currency string) string {
	return fmt.Sprintf("%s:%s", coinID, currency)
}
