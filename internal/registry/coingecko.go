package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func FetchCoinGeckoCatalog(ctx context.Context, baseURL string) ([]CoinEntry, error) {
	url := fmt.Sprintf("%s/coins/list?include_platform=true", strings.TrimRight(baseURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching catalog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog response: %d", resp.StatusCode)
	}

	var entries []CoinEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decoding catalog: %w", err)
	}

	slog.Info("fetched coin catalog", "count", len(entries))
	return entries, nil
}

func StartRefresh(ctx context.Context, reg *TokenRegistry, baseURL string, chains map[string]bool, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				entries, err := FetchCoinGeckoCatalog(ctx, baseURL)
				if err != nil {
					slog.Warn("catalog refresh failed", "error", err)
					continue
				}
				reg.LoadCatalog(entries, chains)
				slog.Info("catalog refreshed", "tokens", reg.Count())
			}
		}
	}()
}
