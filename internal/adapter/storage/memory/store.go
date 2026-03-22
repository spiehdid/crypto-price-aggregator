package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type Store struct {
	mu     sync.RWMutex
	prices map[string][]model.Price // key: "coinID:currency"
}

func NewStore() *Store {
	return &Store{
		prices: make(map[string][]model.Price),
	}
}

func (s *Store) Save(_ context.Context, price *model.Price) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := storeKey(price.CoinID, price.Currency)
	s.prices[key] = append(s.prices[key], *price)
	return nil
}

func (s *Store) SaveBatch(_ context.Context, prices []model.Price) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range prices {
		key := storeKey(p.CoinID, p.Currency)
		s.prices[key] = append(s.prices[key], p)
	}
	return nil
}

func (s *Store) GetLatest(_ context.Context, coinID, currency string) (*model.Price, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.prices[storeKey(coinID, currency)]
	if len(entries) == 0 {
		return nil, nil
	}

	latest := entries[0]
	for _, e := range entries[1:] {
		if e.ReceivedAt.After(latest.ReceivedAt) {
			latest = e
		}
	}

	return &latest, nil
}

func (s *Store) GetHistory(_ context.Context, coinID, currency string, from, to time.Time) ([]model.Price, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.prices[storeKey(coinID, currency)]
	var result []model.Price

	for _, e := range entries {
		if !e.ReceivedAt.Before(from) && e.ReceivedAt.Before(to) {
			result = append(result, e)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ReceivedAt.Before(result[j].ReceivedAt)
	})

	return result, nil
}

func (s *Store) GetOHLC(_ context.Context, coinID, currency string, from, to time.Time, interval time.Duration) ([]model.OHLC, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.prices[storeKey(coinID, currency)]

	var filtered []model.Price
	for _, e := range entries {
		if !e.ReceivedAt.Before(from) && e.ReceivedAt.Before(to) {
			filtered = append(filtered, e)
		}
	}

	if len(filtered) == 0 {
		return nil, nil
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].ReceivedAt.Before(filtered[j].ReceivedAt)
	})

	buckets := make(map[time.Time][]model.Price)
	for _, p := range filtered {
		bucketTime := p.ReceivedAt.Truncate(interval)
		buckets[bucketTime] = append(buckets[bucketTime], p)
	}

	var result []model.OHLC
	for bucketTime, prices := range buckets {
		ohlc := model.OHLC{
			Time:  bucketTime,
			Open:  prices[0].Value,
			High:  prices[0].Value,
			Low:   prices[0].Value,
			Close: prices[len(prices)-1].Value,
		}
		for _, p := range prices {
			if p.Value.GreaterThan(ohlc.High) {
				ohlc.High = p.Value
			}
			if p.Value.LessThan(ohlc.Low) {
				ohlc.Low = p.Value
			}
		}
		result = append(result, ohlc)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.Before(result[j].Time)
	})

	return result, nil
}

func storeKey(coinID, currency string) string {
	return fmt.Sprintf("%s:%s", coinID, currency)
}
