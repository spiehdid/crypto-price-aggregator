package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
)

func (h *Handler) HandleGetPrice(w http.ResponseWriter, r *http.Request) {
	coinID := chi.URLParam(r, "coinID")
	if !isValidCoinID(coinID) {
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid coin ID")
		return
	}

	currency := r.URL.Query().Get("currency")
	if currency == "" {
		currency = h.defaultCurrency
	}

	price, err := h.priceSvc.GetPrice(r.Context(), coinID, currency)
	if err != nil {
		h.handlePriceError(w, err)
		return
	}

	resp := map[string]interface{}{
		"coin":      price.CoinID,
		"currency":  price.Currency,
		"price":     price.Value.String(),
		"provider":  price.Provider,
		"timestamp": price.Timestamp,
	}
	if !price.MarketCap.IsZero() {
		resp["market_cap"] = price.MarketCap.String()
	}
	if !price.Volume24h.IsZero() {
		resp["volume_24h"] = price.Volume24h.String()
	}
	if price.Change24h != 0 {
		resp["change_24h"] = price.Change24h
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) HandleGetPrices(w http.ResponseWriter, r *http.Request) {
	coinsParam := r.URL.Query().Get("coins")
	if coinsParam == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PARAM", "coins query parameter is required")
		return
	}

	currency := r.URL.Query().Get("currency")
	if currency == "" {
		currency = h.defaultCurrency
	}

	coinIDs := splitCoins(coinsParam)
	if len(coinIDs) > 50 {
		writeError(w, http.StatusBadRequest, "TOO_MANY_COINS", "maximum 50 coins per request")
		return
	}
	prices, errs := h.priceSvc.GetPrices(r.Context(), coinIDs, currency)

	errMap := make(map[string]string)
	for k, v := range errs {
		errMap[k] = v.Error()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"prices": prices,
		"errors": errMap,
	})
}

func (h *Handler) HandleGetOHLC(w http.ResponseWriter, r *http.Request) {
	coinID := chi.URLParam(r, "coinID")
	if !isValidCoinID(coinID) {
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid coin ID")
		return
	}

	currency := r.URL.Query().Get("currency")
	if currency == "" {
		currency = h.defaultCurrency
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	intervalStr := r.URL.Query().Get("interval")

	if fromStr == "" || toStr == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PARAM", "from and to query parameters are required")
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", "from must be RFC3339 format")
		return
	}

	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", "to must be RFC3339 format")
		return
	}

	interval := parseInterval(intervalStr)

	candles, err := h.priceSvc.GetOHLC(r.Context(), coinID, currency, from, to, interval)
	if err != nil {
		slog.Error("ohlc query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"coin":     coinID,
		"currency": currency,
		"interval": intervalStr,
		"candles":  candles,
	})
}

func (h *Handler) HandleConvert(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	amountStr := r.URL.Query().Get("amount")

	if from == "" || to == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PARAM", "from and to query parameters are required")
		return
	}

	amount := decimal.NewFromFloat(1)
	if amountStr != "" {
		var err error
		amount, err = decimal.NewFromString(amountStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid amount")
			return
		}
	}

	result, err := h.priceSvc.Convert(r.Context(), from, to, amount, h.defaultCurrency)
	if err != nil {
		h.handlePriceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"from":         result.From,
		"to":           result.To,
		"amount":       result.Amount.String(),
		"result":       result.Result.StringFixed(8),
		"rate":         result.Rate.StringFixed(8),
		"via_currency": result.ViaCurrency,
	})
}

func (h *Handler) HandleGetPriceByAddress(w http.ResponseWriter, r *http.Request) {
	chain := chi.URLParam(r, "chain")
	address := chi.URLParam(r, "address")
	currency := r.URL.Query().Get("currency")
	if currency == "" {
		currency = h.defaultCurrency
	}

	result, err := h.priceSvc.GetPriceByAddress(r.Context(), chain, address, currency)
	if err != nil {
		h.handlePriceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"coin":         result.Price.CoinID,
		"chain":        chain,
		"address":      address,
		"currency":     result.Price.Currency,
		"price":        result.Price.Value.String(),
		"provider":     result.Price.Provider,
		"resolved_via": result.ResolvedVia,
		"timestamp":    result.Price.Timestamp,
	})
}
