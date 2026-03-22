package router

import (
	"sync"
	"time"
)

type CBState int

const (
	StateClosed CBState = iota
	StateOpen
	StateHalfOpen
)

type CircuitBreaker struct {
	mu               sync.Mutex
	state            CBState
	failCount        int
	failureThreshold int
	recoveryTimeout  time.Duration
	lastFailAt       time.Time
}

func NewCircuitBreaker(failureThreshold int, recoveryTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		recoveryTimeout:  recoveryTimeout,
	}
}

func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailAt) > cb.recoveryTimeout {
			cb.state = StateHalfOpen
			return true
		}
		return false
	case StateHalfOpen:
		return true
	}
	return false
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failCount = 0
	cb.state = StateClosed
}

func (cb *CircuitBreaker) RecordFailure(_ error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failCount++
	cb.lastFailAt = time.Now()
	if cb.state == StateHalfOpen || cb.failCount >= cb.failureThreshold {
		cb.state = StateOpen
	}
}

func (cb *CircuitBreaker) State() CBState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == StateOpen && time.Since(cb.lastFailAt) > cb.recoveryTimeout {
		cb.state = StateHalfOpen
	}
	return cb.state
}
