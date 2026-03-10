package influxdb

import (
	"errors"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("influxdb circuit breaker is open")

type CircuitBreakerConfig struct {
	FailureThreshold    int
	OpenTimeout         time.Duration
	HalfOpenMaxRequests int
}

type circuitBreaker struct {
	mu                  sync.Mutex
	state               circuitBreakerState
	consecutiveFailures int
	openedAt            time.Time
	halfOpenInFlight    int
	failureThreshold    int
	openTimeout         time.Duration
	halfOpenMaxRequests int
	clock               func() time.Time
}

type circuitBreakerState uint8

const (
	circuitBreakerClosed circuitBreakerState = iota
	circuitBreakerOpen
	circuitBreakerHalfOpen
)

func newCircuitBreaker(cfg CircuitBreakerConfig) *circuitBreaker {
	cfg = cfg.withDefaults()

	return &circuitBreaker{
		state:               circuitBreakerClosed,
		failureThreshold:    cfg.FailureThreshold,
		openTimeout:         cfg.OpenTimeout,
		halfOpenMaxRequests: cfg.HalfOpenMaxRequests,
		clock:               time.Now,
	}
}

func (c CircuitBreakerConfig) withDefaults() CircuitBreakerConfig {
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 5
	}
	if c.OpenTimeout <= 0 {
		c.OpenTimeout = 30 * time.Second
	}
	if c.HalfOpenMaxRequests <= 0 {
		c.HalfOpenMaxRequests = 1
	}

	return c
}

func (b *circuitBreaker) allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.clock()

	switch b.state {
	case circuitBreakerOpen:
		if now.Sub(b.openedAt) < b.openTimeout {
			return ErrCircuitOpen
		}

		b.state = circuitBreakerHalfOpen
		b.halfOpenInFlight = 0
	case circuitBreakerHalfOpen:
		if b.halfOpenInFlight >= b.halfOpenMaxRequests {
			return ErrCircuitOpen
		}
	}

	if b.state == circuitBreakerHalfOpen {
		b.halfOpenInFlight++
	}

	return nil
}

func (b *circuitBreaker) onSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.state = circuitBreakerClosed
	b.consecutiveFailures = 0
	b.halfOpenInFlight = 0
	b.openedAt = time.Time{}
}

func (b *circuitBreaker) onFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case circuitBreakerHalfOpen:
		b.open()
	case circuitBreakerClosed:
		b.consecutiveFailures++
		if b.consecutiveFailures >= b.failureThreshold {
			b.open()
		}
	case circuitBreakerOpen:
		b.open()
	}
}

func (b *circuitBreaker) open() {
	b.state = circuitBreakerOpen
	b.consecutiveFailures = 0
	b.halfOpenInFlight = 0
	b.openedAt = b.clock()
}
