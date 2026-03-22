package router_test

import (
	"errors"
	"testing"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/router"
	"github.com/stretchr/testify/assert"
)

func TestCircuitBreaker_InitiallyClosed(t *testing.T) {
	cb := router.NewCircuitBreaker(3, 50*time.Millisecond)
	assert.True(t, cb.AllowRequest())
	assert.Equal(t, router.StateClosed, cb.State())
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := router.NewCircuitBreaker(3, 50*time.Millisecond)
	cb.RecordFailure(errors.New("err"))
	cb.RecordFailure(errors.New("err"))
	assert.True(t, cb.AllowRequest())
	cb.RecordFailure(errors.New("err"))
	assert.False(t, cb.AllowRequest())
	assert.Equal(t, router.StateOpen, cb.State())
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := router.NewCircuitBreaker(2, 50*time.Millisecond)
	cb.RecordFailure(errors.New("err"))
	cb.RecordFailure(errors.New("err"))
	assert.False(t, cb.AllowRequest())
	time.Sleep(60 * time.Millisecond)
	assert.True(t, cb.AllowRequest())
	assert.Equal(t, router.StateHalfOpen, cb.State())
}

func TestCircuitBreaker_ClosesOnSuccessFromHalfOpen(t *testing.T) {
	cb := router.NewCircuitBreaker(2, 50*time.Millisecond)
	cb.RecordFailure(errors.New("err"))
	cb.RecordFailure(errors.New("err"))
	time.Sleep(60 * time.Millisecond)
	cb.AllowRequest()
	cb.RecordSuccess()
	assert.Equal(t, router.StateClosed, cb.State())
	assert.True(t, cb.AllowRequest())
}

func TestCircuitBreaker_ReopensOnFailureFromHalfOpen(t *testing.T) {
	cb := router.NewCircuitBreaker(2, 50*time.Millisecond)
	cb.RecordFailure(errors.New("err"))
	cb.RecordFailure(errors.New("err"))
	time.Sleep(60 * time.Millisecond)
	cb.AllowRequest()
	cb.RecordFailure(errors.New("err"))
	assert.Equal(t, router.StateOpen, cb.State())
	assert.False(t, cb.AllowRequest())
}

func TestCircuitBreaker_SuccessResetsFails(t *testing.T) {
	cb := router.NewCircuitBreaker(3, 50*time.Millisecond)
	cb.RecordFailure(errors.New("err"))
	cb.RecordFailure(errors.New("err"))
	cb.RecordSuccess()
	cb.RecordFailure(errors.New("err"))
	assert.True(t, cb.AllowRequest())
}
