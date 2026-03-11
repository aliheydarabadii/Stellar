package influxdb

import (
	"errors"
	"testing"
	"time"

	gobreaker "github.com/sony/gobreaker/v2"

	"stellar/internal/measurements/app/query"
)

func TestCircuitBreakerOpensAfterFailureThreshold(t *testing.T) {
	t.Parallel()

	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    2,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	reportFailure(t, breaker)
	if breaker.state() != gobreaker.StateClosed {
		t.Fatalf("expected breaker to remain closed after first failure, got %v", breaker.state())
	}

	reportFailure(t, breaker)
	if breaker.state() != gobreaker.StateOpen {
		t.Fatalf("expected breaker to open after threshold, got %v", breaker.state())
	}
}

func TestCircuitBreakerBlocksRequestsWhileOpen(t *testing.T) {
	t.Parallel()

	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	reportFailure(t, breaker)

	_, err := breaker.allow()
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreakerTransitionsToHalfOpenAfterTimeout(t *testing.T) {
	t.Parallel()

	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	reportFailure(t, breaker)
	waitForBreakerTimeout(t, 20*time.Millisecond)

	done, err := breaker.allow()
	if err != nil {
		t.Fatalf("expected half-open trial request to be allowed, got %v", err)
	}
	if breaker.state() != gobreaker.StateHalfOpen {
		t.Fatalf("expected breaker to transition to half-open, got %v", breaker.state())
	}

	done(nil)
}

func TestCircuitBreakerClosesAgainAfterSuccessfulHalfOpenTrial(t *testing.T) {
	t.Parallel()

	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	reportFailure(t, breaker)
	waitForBreakerTimeout(t, 20*time.Millisecond)

	done, err := breaker.allow()
	if err != nil {
		t.Fatalf("expected half-open trial request to be allowed, got %v", err)
	}

	done(nil)

	if breaker.state() != gobreaker.StateClosed {
		t.Fatalf("expected breaker to close after success, got %v", breaker.state())
	}
}

func TestCircuitBreakerRequiresAllHalfOpenProbeSuccessesBeforeClosing(t *testing.T) {
	t.Parallel()

	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	})

	reportFailure(t, breaker)
	waitForBreakerTimeout(t, 20*time.Millisecond)

	done1, err := breaker.allow()
	if err != nil {
		t.Fatalf("expected first half-open probe to be allowed, got %v", err)
	}
	done2, err := breaker.allow()
	if err != nil {
		t.Fatalf("expected second half-open probe to be allowed, got %v", err)
	}
	if _, err := breaker.allow(); !errors.Is(err, gobreaker.ErrTooManyRequests) {
		t.Fatalf("expected third half-open probe to be rejected, got %v", err)
	}

	done1(nil)
	if breaker.state() != gobreaker.StateHalfOpen {
		t.Fatalf("expected breaker to remain half-open after first success, got %v", breaker.state())
	}

	done2(nil)
	if breaker.state() != gobreaker.StateClosed {
		t.Fatalf("expected breaker to close after second success, got %v", breaker.state())
	}
}

func TestCircuitBreakerFailureCountResetsAfterSuccess(t *testing.T) {
	t.Parallel()

	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    3,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	reportFailure(t, breaker)
	reportFailure(t, breaker)
	if breaker.counts().ConsecutiveFailures != 2 {
		t.Fatalf("expected two consecutive failures, got %d", breaker.counts().ConsecutiveFailures)
	}

	reportSuccess(t, breaker)
	if breaker.counts().ConsecutiveFailures != 0 {
		t.Fatalf("expected failure count reset after success, got %d", breaker.counts().ConsecutiveFailures)
	}
	if breaker.counts().ConsecutiveSuccesses != 1 {
		t.Fatalf("expected one consecutive success, got %d", breaker.counts().ConsecutiveSuccesses)
	}
}

func TestCircuitBreakerExcludesNonDependencyErrors(t *testing.T) {
	t.Parallel()

	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	done, err := breaker.allow()
	if err != nil {
		t.Fatalf("expected breaker to allow request, got %v", err)
	}

	done(errors.New("mapping failed"))

	if breaker.state() != gobreaker.StateClosed {
		t.Fatalf("expected breaker to remain closed after excluded error, got %v", breaker.state())
	}
	counts := breaker.counts()
	if counts.TotalFailures != 0 || counts.ConsecutiveFailures != 0 {
		t.Fatalf("expected excluded error not to count as a failure, got %+v", counts)
	}
	if counts.TotalExclusions != 1 {
		t.Fatalf("expected one exclusion to be recorded, got %+v", counts)
	}
}

func newTestCircuitBreaker(cfg CircuitBreakerConfig) *circuitBreaker {
	return newCircuitBreaker(cfg)
}

func reportFailure(t *testing.T, breaker *circuitBreaker) {
	t.Helper()

	done, err := breaker.allow()
	if err != nil {
		t.Fatalf("expected breaker to allow request, got %v", err)
	}

	done(query.ErrReadModelUnavailable)
}

func reportSuccess(t *testing.T, breaker *circuitBreaker) {
	t.Helper()

	done, err := breaker.allow()
	if err != nil {
		t.Fatalf("expected breaker to allow request, got %v", err)
	}

	done(nil)
}

func waitForBreakerTimeout(t *testing.T, timeout time.Duration) {
	t.Helper()

	time.Sleep(timeout + 20*time.Millisecond)
}
