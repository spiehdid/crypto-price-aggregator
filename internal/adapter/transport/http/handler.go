package http

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type Handler struct {
	priceSvc        *app.PriceService
	adminSvc        *app.AdminService
	alertSvc        *app.AlertService
	analyticsSvc    *app.AnalyticsService
	defaultCurrency string
}

func NewHandler(priceSvc *app.PriceService, adminSvc *app.AdminService, alertSvc *app.AlertService, analyticsSvc *app.AnalyticsService, defaultCurrency string) *Handler {
	return &Handler{
		priceSvc:        priceSvc,
		adminSvc:        adminSvc,
		alertSvc:        alertSvc,
		analyticsSvc:    analyticsSvc,
		defaultCurrency: defaultCurrency,
	}
}

func isValidCoinID(s string) bool {
	if len(s) == 0 || len(s) > 100 {
		return false
	}
	for _, c := range s {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' && c != '_' && c != '.' {
			return false
		}
	}
	return true
}

func splitCoins(s string) []string {
	var coins []string
	for _, c := range strings.Split(s, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			coins = append(coins, c)
		}
	}
	return coins
}

func (h *Handler) handlePriceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, model.ErrCoinNotFound):
		writeError(w, http.StatusNotFound, "COIN_NOT_FOUND", err.Error())
	case errors.Is(err, model.ErrNoHealthyProvider):
		writeError(w, http.StatusServiceUnavailable, "NO_HEALTHY_PROVIDER", err.Error())
	case errors.Is(err, model.ErrBudgetExhausted):
		writeError(w, http.StatusServiceUnavailable, "BUDGET_EXHAUSTED", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func parseTimeRange(r *http.Request) (time.Time, time.Time) {
	now := time.Now()
	from := now.Add(-24 * time.Hour) // default: last 24 hours
	to := now

	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	return from, to
}

func parseInterval(s string) time.Duration {
	switch s {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	case "4h":
		return 4 * time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return time.Hour
	}
}
