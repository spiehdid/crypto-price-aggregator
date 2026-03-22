package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type createAlertRequest struct {
	CoinID     string `json:"coin_id"`
	Currency   string `json:"currency"`
	Condition  string `json:"condition"` // "above" or "below"
	Threshold  string `json:"threshold"`
	WebhookURL string `json:"webhook_url"`
}

func (h *Handler) HandleCreateAlert(w http.ResponseWriter, r *http.Request) {
	var req createAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}

	if req.CoinID == "" || req.Threshold == "" || req.WebhookURL == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PARAM", "coin_id, threshold, and webhook_url are required")
		return
	}

	threshold, err := decimal.NewFromString(req.Threshold)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid threshold value")
		return
	}

	condition := model.ConditionAbove
	if req.Condition == "below" {
		condition = model.ConditionBelow
	}

	currency := req.Currency
	if currency == "" {
		currency = h.defaultCurrency
	}

	alert := &model.Alert{
		CoinID:     req.CoinID,
		Currency:   currency,
		Condition:  condition,
		Threshold:  threshold,
		WebhookURL: req.WebhookURL,
	}

	if err := h.alertSvc.Create(alert); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WEBHOOK_URL", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, alert)
}

func (h *Handler) HandleListAlerts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"alerts": h.alertSvc.List(),
	})
}

func (h *Handler) HandleDeleteAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.alertSvc.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, "ALERT_NOT_FOUND", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
