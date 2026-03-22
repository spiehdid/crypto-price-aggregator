package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type Config struct {
	Addr     string
	Password string
	DB       int
}

type Cache struct {
	client *goredis.Client
}

func NewCache(ctx context.Context, cfg Config) (*Cache, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}

	return &Cache{client: client}, nil
}

func (c *Cache) Close() error {
	return c.client.Close()
}

type priceJSON struct {
	CoinID     string  `json:"coin_id"`
	Currency   string  `json:"currency"`
	Value      string  `json:"value"`
	Provider   string  `json:"provider"`
	Timestamp  int64   `json:"timestamp"`
	ReceivedAt int64   `json:"received_at"`
	MarketCap  string  `json:"market_cap"`
	Volume24h  string  `json:"volume_24h"`
	Change24h  float64 `json:"change_24h"`
}

func (c *Cache) Get(ctx context.Context, coinID, currency string) (*model.Price, error) {
	data, err := c.client.Get(ctx, cacheKey(coinID, currency)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var pj priceJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		// Delete corrupted key
		_ = c.client.Del(ctx, cacheKey(coinID, currency)).Err()
		return nil, fmt.Errorf("unmarshaling price (key deleted): %w", err)
	}

	value, err := decimal.NewFromString(pj.Value)
	if err != nil {
		return nil, fmt.Errorf("parsing decimal: %w", err)
	}

	mc, _ := decimal.NewFromString(pj.MarketCap)
	vol, _ := decimal.NewFromString(pj.Volume24h)

	return &model.Price{
		CoinID:     pj.CoinID,
		Currency:   pj.Currency,
		Value:      value,
		Provider:   pj.Provider,
		Timestamp:  time.UnixMilli(pj.Timestamp),
		ReceivedAt: time.UnixMilli(pj.ReceivedAt),
		MarketCap:  mc,
		Volume24h:  vol,
		Change24h:  pj.Change24h,
	}, nil
}

func (c *Cache) Set(ctx context.Context, price *model.Price, ttl time.Duration) error {
	pj := priceJSON{
		CoinID:     price.CoinID,
		Currency:   price.Currency,
		Value:      price.Value.String(),
		Provider:   price.Provider,
		Timestamp:  price.Timestamp.UnixMilli(),
		ReceivedAt: price.ReceivedAt.UnixMilli(),
		MarketCap:  price.MarketCap.String(),
		Volume24h:  price.Volume24h.String(),
		Change24h:  price.Change24h,
	}

	data, err := json.Marshal(pj)
	if err != nil {
		return fmt.Errorf("marshaling price: %w", err)
	}

	return c.client.Set(ctx, cacheKey(price.CoinID, price.Currency), data, ttl).Err()
}

func (c *Cache) Delete(ctx context.Context, coinID, currency string) error {
	return c.client.Del(ctx, cacheKey(coinID, currency)).Err()
}

func cacheKey(coinID, currency string) string {
	return fmt.Sprintf("price:%s:%s", coinID, currency)
}
