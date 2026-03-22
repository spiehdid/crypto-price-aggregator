package http

import (
	"log/slog"
	"net/http"
	"strconv"
)

func (h *Handler) HandleProviderAccuracy(w http.ResponseWriter, r *http.Request) {
	if h.analyticsSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "ANALYTICS_DISABLED", "analytics not configured")
		return
	}
	from, to := parseTimeRange(r)
	result, err := h.analyticsSvc.GetProviderAccuracy(r.Context(), from, to)
	if err != nil {
		slog.Error("analytics query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"providers": result})
}

func (h *Handler) HandlePriceAnomalies(w http.ResponseWriter, r *http.Request) {
	if h.analyticsSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "ANALYTICS_DISABLED", "analytics not configured")
		return
	}
	from, to := parseTimeRange(r)
	coinID := r.URL.Query().Get("coin")
	currency := r.URL.Query().Get("currency")
	if currency == "" {
		currency = h.defaultCurrency
	}
	deviation := 5.0 // default
	if v := r.URL.Query().Get("deviation"); v != "" {
		if d, err := strconv.ParseFloat(v, 64); err == nil {
			deviation = d
		}
	}
	result, err := h.analyticsSvc.GetAnomalies(r.Context(), coinID, currency, deviation, from, to)
	if err != nil {
		slog.Error("analytics query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"anomalies": result})
}

func (h *Handler) HandleVolumeByProvider(w http.ResponseWriter, r *http.Request) {
	if h.analyticsSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "ANALYTICS_DISABLED", "analytics not configured")
		return
	}
	from, to := parseTimeRange(r)
	result, err := h.analyticsSvc.GetVolumeByProvider(r.Context(), from, to)
	if err != nil {
		slog.Error("analytics query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"providers": result})
}

func (h *Handler) HandlePriceStats(w http.ResponseWriter, r *http.Request) {
	if h.analyticsSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "ANALYTICS_DISABLED", "analytics not configured")
		return
	}
	from, to := parseTimeRange(r)
	coinID := r.URL.Query().Get("coin")
	currency := r.URL.Query().Get("currency")
	if currency == "" {
		currency = h.defaultCurrency
	}
	if coinID == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PARAM", "coin query parameter required")
		return
	}
	result, err := h.analyticsSvc.GetPriceStats(r.Context(), coinID, currency, from, to)
	if err != nil {
		slog.Error("analytics query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
