package influxdb

import (
	"errors"
	getmeasurements "stellar/internal/measurements/application"
	"testing"
	"time"

	gobreaker "github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/suite"
)

type CircuitBreakerSuite struct {
	suite.Suite
}

func TestCircuitBreakerSuite(t *testing.T) {
	suite.Run(t, new(CircuitBreakerSuite))
}

func (s *CircuitBreakerSuite) TestOpensAfterFailureThreshold() {
	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    2,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	s.reportFailure(breaker)
	s.Equal(gobreaker.StateClosed, breaker.state())

	s.reportFailure(breaker)
	s.Equal(gobreaker.StateOpen, breaker.state())
}

func (s *CircuitBreakerSuite) TestBlocksRequestsWhileOpen() {
	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	s.reportFailure(breaker)

	_, err := breaker.allow()
	s.ErrorIs(err, ErrCircuitOpen)
}

func (s *CircuitBreakerSuite) TestTransitionsToHalfOpenAfterTimeout() {
	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	s.reportFailure(breaker)
	waitForBreakerTimeout(20 * time.Millisecond)

	done, err := breaker.allow()
	s.Require().NoError(err)
	s.Equal(gobreaker.StateHalfOpen, breaker.state())

	done(nil)
}

func (s *CircuitBreakerSuite) TestClosesAgainAfterSuccessfulHalfOpenTrial() {
	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	s.reportFailure(breaker)
	waitForBreakerTimeout(20 * time.Millisecond)

	done, err := breaker.allow()
	s.Require().NoError(err)

	done(nil)

	s.Equal(gobreaker.StateClosed, breaker.state())
}

func (s *CircuitBreakerSuite) TestRequiresAllHalfOpenProbeSuccessesBeforeClosing() {
	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	})

	s.reportFailure(breaker)
	waitForBreakerTimeout(20 * time.Millisecond)

	done1, err := breaker.allow()
	s.Require().NoError(err)
	done2, err := breaker.allow()
	s.Require().NoError(err)
	_, err = breaker.allow()
	s.ErrorIs(err, gobreaker.ErrTooManyRequests)

	done1(nil)
	s.Equal(gobreaker.StateHalfOpen, breaker.state())

	done2(nil)
	s.Equal(gobreaker.StateClosed, breaker.state())
}

func (s *CircuitBreakerSuite) TestFailureCountResetsAfterSuccess() {
	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    3,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	s.reportFailure(breaker)
	s.reportFailure(breaker)
	s.Equal(uint32(2), breaker.counts().ConsecutiveFailures)

	s.reportSuccess(breaker)
	s.Equal(uint32(0), breaker.counts().ConsecutiveFailures)
	s.Equal(uint32(1), breaker.counts().ConsecutiveSuccesses)
}

func (s *CircuitBreakerSuite) TestExcludesNonDependencyErrors() {
	breaker := newTestCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    1,
		OpenTimeout:         20 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	done, err := breaker.allow()
	s.Require().NoError(err)

	done(errors.New("mapping failed"))

	s.Equal(gobreaker.StateClosed, breaker.state())
	counts := breaker.counts()
	s.Zero(counts.TotalFailures)
	s.Zero(counts.ConsecutiveFailures)
	s.Equal(uint32(1), counts.TotalExclusions)
}

func (s *CircuitBreakerSuite) reportFailure(breaker *circuitBreaker) {
	s.T().Helper()

	done, err := breaker.allow()
	s.Require().NoError(err)

	done(getmeasurements.ErrReadModelUnavailable)
}

func (s *CircuitBreakerSuite) reportSuccess(breaker *circuitBreaker) {
	s.T().Helper()

	done, err := breaker.allow()
	s.Require().NoError(err)

	done(nil)
}

func newTestCircuitBreaker(cfg CircuitBreakerConfig) *circuitBreaker {
	return newCircuitBreaker(cfg)
}

func waitForBreakerTimeout(timeout time.Duration) {
	time.Sleep(timeout + 20*time.Millisecond)
}
