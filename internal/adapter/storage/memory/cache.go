package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type cacheEntry struct {
	price     *model.Price
	expiresAt time.Time
}

type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

func NewCache() *Cache {
	return &Cache{
		entries: make(map[string]cacheEntry),
	}
}

func (c *Cache) Get(_ context.Context, coinID, currency string) (*model.Price, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[cacheKey(coinID, currency)]
	if !ok {
		return nil, nil
	}

	if time.Now().After(entry.expiresAt) {
		return nil, nil
	}

	return entry.price, nil
}

func (c *Cache) Set(_ context.Context, price *model.Price, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[cacheKey(price.CoinID, price.Currency)] = cacheEntry{
		price:     price,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (c *Cache) Delete(_ context.Context, coinID, currency string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, cacheKey(coinID, currency))
	return nil
}

func cacheKey(coinID, currency string) string {
	return fmt.Sprintf("%s:%s", coinID, currency)
}
