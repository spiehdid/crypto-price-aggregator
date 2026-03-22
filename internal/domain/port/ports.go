package port

import (
	"context"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

// PriceProvider fetches prices from an external API.
type PriceProvider interface {
	GetPrice(ctx context.Context, coinID, currency string) (*model.Price, error)
	GetPrices(ctx context.Context, coinIDs []string, currency string) ([]model.Price, error)
	Name() string
	Status() *model.ProviderStatus
}

// StreamProvider delivers realtime price updates via channel.
type StreamProvider interface {
	Subscribe(ctx context.Context, coinIDs []string, currency string) (<-chan model.Price, error)
	Close() error
}

// PriceStore persists prices for historical queries.
type PriceStore interface {
	Save(ctx context.Context, price *model.Price) error
	SaveBatch(ctx context.Context, prices []model.Price) error
	GetLatest(ctx context.Context, coinID, currency string) (*model.Price, error)
	GetHistory(ctx context.Context, coinID, currency string, from, to time.Time) ([]model.Price, error)
	GetOHLC(ctx context.Context, coinID, currency string, from, to time.Time, interval time.Duration) ([]model.OHLC, error)
}

// Cache provides fast price lookups with TTL-based expiry.
type Cache interface {
	Get(ctx context.Context, coinID, currency string) (*model.Price, error)
	Set(ctx context.Context, price *model.Price, ttl time.Duration) error
	Delete(ctx context.Context, coinID, currency string) error
}

// ProviderRouter selects the best provider for a given request.
type ProviderRouter interface {
	Select(ctx context.Context, coinID string) (PriceProvider, error)
	ReportSuccess(provider string, latency time.Duration)
	ReportFailure(provider string, err error)
}

// TokenPriceProvider can look up price by contract address directly.
type TokenPriceProvider interface {
	GetPriceByAddress(ctx context.Context, chain, address, currency string) (*model.Price, error)
	SupportsChain(chain string) bool
}

// AnalyticsStore provides analytical queries over price history.
type AnalyticsStore interface {
	ProviderAccuracy(ctx context.Context, from, to time.Time) ([]model.ProviderAccuracyReport, error)
	PriceAnomalies(ctx context.Context, coinID, currency string, deviationPercent float64, from, to time.Time) ([]model.PriceAnomaly, error)
	VolumeByProvider(ctx context.Context, from, to time.Time) ([]model.ProviderVolume, error)
	PriceStats(ctx context.Context, coinID, currency string, from, to time.Time) (*model.PriceStatsSummary, error)
}
