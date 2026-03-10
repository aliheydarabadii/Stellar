package influxdb

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerOpensAfterFailureThreshold(t *testing.T) {
	t.Parallel()

	breaker, clock := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    2,
		OpenTimeout:         time.Minute,
		HalfOpenMaxRequests: 1,
	})

	if err := breaker.allow(); err != nil {
		t.Fatalf("expected closed breaker to allow, got %v", err)
	}

	breaker.onFailure()
	if breaker.state != circuitBreakerClosed {
		t.Fatalf("expected breaker to remain closed after first failure, got %v", breaker.state)
	}

	breaker.onFailure()
	if breaker.state != circuitBreakerOpen {
		t.Fatalf("expected breaker to open after threshold, got %v", breaker.state)
	}
	if !breaker.openedAt.Equal(*clock) {
		t.Fatalf("expected openedAt %v, got %v", *clock, breaker.openedAt)
	}
}

func TestCircuitBreakerBlocksRequestsWhileOpen(t *testing.T) {
	t.Parallel()

	breaker, _ := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         time.Minute,
		HalfOpenMaxRequests: 1,
	})

	breaker.onFailure()

	err := breaker.allow()
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreakerTransitionsToHalfOpenAfterTimeout(t *testing.T) {
	t.Parallel()

	breaker, clock := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         10 * time.Second,
		HalfOpenMaxRequests: 1,
	})

	breaker.onFailure()
	*clock = (*clock).Add(11 * time.Second)

	if err := breaker.allow(); err != nil {
		t.Fatalf("expected half-open trial request to be allowed, got %v", err)
	}
	if breaker.state != circuitBreakerHalfOpen {
		t.Fatalf("expected breaker to transition to half-open, got %v", breaker.state)
	}
	if breaker.halfOpenInFlight != 1 {
		t.Fatalf("expected one in-flight half-open request, got %d", breaker.halfOpenInFlight)
	}
}

func TestCircuitBreakerClosesAgainAfterSuccessfulHalfOpenTrial(t *testing.T) {
	t.Parallel()

	breaker, clock := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         10 * time.Second,
		HalfOpenMaxRequests: 1,
	})

	breaker.onFailure()
	*clock = (*clock).Add(11 * time.Second)

	if err := breaker.allow(); err != nil {
		t.Fatalf("expected half-open trial request to be allowed, got %v", err)
	}

	breaker.onSuccess()

	if breaker.state != circuitBreakerClosed {
		t.Fatalf("expected breaker to close after success, got %v", breaker.state)
	}
	if breaker.halfOpenInFlight != 0 {
		t.Fatalf("expected half-open in-flight count to reset, got %d", breaker.halfOpenInFlight)
	}
	if breaker.consecutiveFailures != 0 {
		t.Fatalf("expected consecutive failures to reset, got %d", breaker.consecutiveFailures)
	}
}

func TestCircuitBreakerFailureCountResetsAfterSuccess(t *testing.T) {
	t.Parallel()

	breaker, _ := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    3,
		OpenTimeout:         time.Minute,
		HalfOpenMaxRequests: 1,
	})

	breaker.onFailure()
	breaker.onFailure()
	if breaker.consecutiveFailures != 2 {
		t.Fatalf("expected two consecutive failures, got %d", breaker.consecutiveFailures)
	}

	breaker.onSuccess()
	if breaker.consecutiveFailures != 0 {
		t.Fatalf("expected failure count reset after success, got %d", breaker.consecutiveFailures)
	}

	breaker.onFailure()
	if breaker.state != circuitBreakerClosed {
		t.Fatalf("expected breaker to stay closed after first failure post-reset, got %v", breaker.state)
	}
	if breaker.consecutiveFailures != 1 {
		t.Fatalf("expected failure count to restart at 1, got %d", breaker.consecutiveFailures)
	}
}

func newTestCircuitBreaker(cfg CircuitBreakerConfig) (*circuitBreaker, *time.Time) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	breaker := newCircuitBreaker(cfg)
	breaker.clock = func() time.Time { return now }
	return breaker, &now
}
