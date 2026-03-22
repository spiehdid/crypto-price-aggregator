package common

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

// BaseProvider provides shared functionality for all price provider adapters.
type BaseProvider struct {
	ProviderName string
	Client       *http.Client
	Tier         model.ProviderTier

	mu            sync.RWMutex
	healthy       bool
	lastError     error
	lastSuccessAt time.Time
}

func NewBaseProvider(name string, tier model.ProviderTier) BaseProvider {
	return BaseProvider{
		ProviderName: name,
		Client:       &http.Client{Timeout: 10 * time.Second},
		Tier:         tier,
		healthy:      true,
	}
}

func (b *BaseProvider) Name() string { return b.ProviderName }

func (b *BaseProvider) Status() *model.ProviderStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return &model.ProviderStatus{
		Name:          b.ProviderName,
		Healthy:       b.healthy,
		Tier:          b.Tier,
		LastError:     b.lastError,
		LastSuccessAt: b.lastSuccessAt,
	}
}

func (b *BaseProvider) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.healthy = true
	b.lastError = nil
	b.lastSuccessAt = time.Now()
}

func (b *BaseProvider) RecordError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.healthy = false
	b.lastError = err
}

// DoRequest makes an HTTP GET request and returns the response. Handles common errors.
func (b *BaseProvider) DoRequest(ctx context.Context, url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := b.Client.Do(req)
	if err != nil {
		b.RecordError(model.ErrProviderDown)
		return nil, model.ErrProviderDown
	}

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		_ = resp.Body.Close()
		b.RecordError(model.ErrRateLimited)
		return nil, model.ErrRateLimited
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		_ = resp.Body.Close()
		b.RecordError(model.ErrUnauthorized)
		return nil, model.ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		_ = resp.Body.Close()
		b.RecordError(model.ErrCoinNotFound)
		return nil, model.ErrCoinNotFound
	case resp.StatusCode >= 500:
		_ = resp.Body.Close()
		b.RecordError(model.ErrProviderDown)
		return nil, model.ErrProviderDown
	case resp.StatusCode != http.StatusOK:
		_ = resp.Body.Close()
		err := fmt.Errorf("unexpected status: %d", resp.StatusCode)
		b.RecordError(err)
		return nil, err
	}

	return resp, nil
}

// DecodeJSON decodes JSON response body using json.Number to preserve decimal precision.
func (b *BaseProvider) DecodeJSON(body io.Reader, target interface{}) error {
	dec := json.NewDecoder(body)
	dec.UseNumber()
	return dec.Decode(target)
}
