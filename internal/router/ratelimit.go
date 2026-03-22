package router

import (
	"sync"
	"time"
)

type RateLimitTracker struct {
	mu        sync.Mutex
	limit     int
	remaining int
	window    time.Duration
	resetAt   time.Time
}

func NewRateLimitTracker(limit int, window time.Duration) *RateLimitTracker {
	return &RateLimitTracker{
		limit:     limit,
		remaining: limit,
		window:    window,
		resetAt:   time.Now().Add(window),
	}
}

func (r *RateLimitTracker) Consume() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkReset()
	if r.remaining > 0 {
		r.remaining--
	}
}

func (r *RateLimitTracker) Remaining() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkReset()
	return r.remaining
}

func (r *RateLimitTracker) IsExhausted() bool {
	return r.Remaining() <= 0
}

func (r *RateLimitTracker) RemainingRatio() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkReset()
	if r.limit == 0 {
		return 0
	}
	return float64(r.remaining) / float64(r.limit)
}

func (r *RateLimitTracker) checkReset() {
	if time.Now().After(r.resetAt) {
		r.remaining = r.limit
		r.resetAt = time.Now().Add(r.window)
	}
}
