package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type RouterConfig struct {
	AdminAPIKey      string
	RateLimitEnabled bool
	RateLimitRPS     float64
	RateLimitBurst   int
	WSHandler        http.HandlerFunc
	CORSEnabled      bool
	CORSOrigins      []string
	ForceHTTPS       bool
}

func NewRouter(h *Handler, cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	if cfg.CORSEnabled {
		r.Use(CORSMiddleware(cfg.CORSOrigins))
	}

	r.Use(SecurityHeadersMiddleware(cfg.ForceHTTPS))
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	if cfg.RateLimitEnabled {
		r.Use(RateLimitMiddleware(cfg.RateLimitRPS, cfg.RateLimitBurst))
	}

	r.Get("/healthz", h.HandleHealthz)
	r.Get("/readyz", h.HandleReadyz)
	r.Get("/swagger", HandleSwaggerUI)
	r.Get("/swagger/openapi.yaml", HandleSwaggerSpec)
	r.Mount("/dashboard", http.StripPrefix("/dashboard", DashboardHandler()))

	if cfg.WSHandler != nil {
		r.Get("/ws/prices", cfg.WSHandler)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/price/{coinID}", h.HandleGetPrice)
		r.Get("/price/address/{chain}/{address}", h.HandleGetPriceByAddress)
		r.Get("/prices", h.HandleGetPrices)
		r.Get("/ohlc/{coinID}", h.HandleGetOHLC)
		r.Get("/convert", h.HandleConvert)
		r.Post("/alerts", h.HandleCreateAlert)
		r.Get("/alerts", h.HandleListAlerts)
		r.Delete("/alerts/{id}", h.HandleDeleteAlert)

		r.Route("/analytics", func(r chi.Router) {
			r.Get("/provider-accuracy", h.HandleProviderAccuracy)
			r.Get("/anomalies", h.HandlePriceAnomalies)
			r.Get("/volume-by-provider", h.HandleVolumeByProvider)
			r.Get("/price-stats", h.HandlePriceStats)
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(AuthMiddleware(cfg.AdminAPIKey))
			r.Get("/providers", h.HandleGetProviders)
			r.Get("/subscriptions", h.HandleGetSubscriptions)
			r.Post("/subscriptions", h.HandlePostSubscription)
			r.Delete("/subscriptions/{coinID}", h.HandleDeleteSubscription)
			r.Get("/stats", h.HandleGetStats)
		})
	})

	return r
}
