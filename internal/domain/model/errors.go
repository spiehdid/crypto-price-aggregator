package model

import "errors"

var (
	ErrRateLimited  = errors.New("rate limit exceeded")
	ErrProviderDown = errors.New("provider unavailable")
	ErrCoinNotFound = errors.New("coin not found")
	ErrUnauthorized = errors.New("invalid api key")
)

var (
	ErrNoHealthyProvider = errors.New("no healthy provider available")
	ErrBudgetExhausted   = errors.New("monthly paid budget exhausted")
	ErrStoreUnavailable  = errors.New("storage unavailable")
)
