package influxdb

import (
	"errors"
	getmeasurements "stellar/internal/measurements/application"
	"time"

	"github.com/sony/gobreaker/v2"
)

var ErrCircuitOpen = gobreaker.ErrOpenState

type CircuitBreakerConfig struct {
	FailureThreshold    int
	OpenTimeout         time.Duration
	HalfOpenMaxRequests int
}

type circuitBreaker struct {
	breaker *gobreaker.TwoStepCircuitBreaker[struct{}]
}

func newCircuitBreaker(cfg CircuitBreakerConfig) *circuitBreaker {
	cfg = cfg.withDefaults()

	return &circuitBreaker{
		breaker: gobreaker.NewTwoStepCircuitBreaker[struct{}](gobreaker.Settings{
			Name:        "measurements-influxdb",
			MaxRequests: uint32(cfg.HalfOpenMaxRequests),
			Timeout:     cfg.OpenTimeout,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= uint32(cfg.FailureThreshold)
			},
			IsExcluded: func(err error) bool {
				return err != nil && !errors.Is(err, getmeasurements.ErrReadModelUnavailable)
			},
		}),
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

func (b *circuitBreaker) allow() (func(error), error) {
	return b.breaker.Allow()
}

func (b *circuitBreaker) state() gobreaker.State {
	return b.breaker.State()
}

func (b *circuitBreaker) counts() gobreaker.Counts {
	return b.breaker.Counts()
}
