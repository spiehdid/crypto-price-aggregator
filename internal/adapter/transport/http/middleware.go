package http

import (
	"crypto/subtle"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"
)

func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			allowed := len(allowedOrigins) == 0 // empty = allow all
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func AuthMiddleware(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("X-API-Key")
			if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type ipLimiter struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

func SecurityHeadersMiddleware(forceHTTPS bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			if forceHTTPS {
				w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			if reqID := middleware.GetReqID(r.Context()); reqID != "" {
				w.Header().Set("X-Request-ID", reqID)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RateLimitMiddleware(rps float64, burst int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	limiters := make(map[string]*ipLimiter)

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			mu.Lock()
			for ip, lim := range limiters {
				if time.Since(lim.lastAccess) > 10*time.Minute {
					delete(limiters, ip)
				}
			}
			mu.Unlock()
		}
	}()

	getLimiter := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()

		if lim, ok := limiters[ip]; ok {
			lim.lastAccess = time.Now()
			return lim.limiter
		}

		lim := &ipLimiter{
			limiter:    rate.NewLimiter(rate.Limit(rps), burst),
			lastAccess: time.Now(),
		}
		limiters[ip] = lim
		return lim.limiter
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}

			if !getLimiter(ip).Allow() {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
