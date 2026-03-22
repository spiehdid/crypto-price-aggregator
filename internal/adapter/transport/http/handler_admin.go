package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) HandleGetProviders(w http.ResponseWriter, r *http.Request) {
	statuses := h.adminSvc.GetProviderStatuses()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"providers": statuses,
	})
}

func (h *Handler) HandleGetSubscriptions(w http.ResponseWriter, r *http.Request) {
	subs := h.adminSvc.GetSubscriptions()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscriptions": subs,
	})
}

type subscribeRequest struct {
	CoinID   string `json:"coin_id"`
	Currency string `json:"currency"`
	Interval string `json:"interval"` // e.g. "30s", "1m"
}

func (h *Handler) HandlePostSubscription(w http.ResponseWriter, r *http.Request) {
	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}

	if req.CoinID == "" || req.Currency == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PARAM", "coin_id and currency are required")
		return
	}

	interval, err := time.ParseDuration(req.Interval)
	if err != nil {
		interval = 60 * time.Second // default
	}

	if err := h.adminSvc.AddSubscription(r.Context(), req.CoinID, req.Currency, interval); err != nil {
		slog.Error("add subscription failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "subscribed"})
}

func (h *Handler) HandleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	coinID := chi.URLParam(r, "coinID")
	currency := r.URL.Query().Get("currency")
	if currency == "" {
		currency = h.defaultCurrency
	}

	if err := h.adminSvc.RemoveSubscription(coinID, currency); err != nil {
		writeError(w, http.StatusNotFound, "SUBSCRIPTION_NOT_FOUND", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}

func (h *Handler) HandleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := h.adminSvc.GetStats()
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	statuses := h.adminSvc.GetProviderStatuses()
	healthyCount := 0
	for _, s := range statuses {
		if s.Healthy {
			healthyCount++
		}
	}

	if healthyCount == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"status":    "not ready",
			"providers": 0,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ready",
		"providers": healthyCount,
	})
}
