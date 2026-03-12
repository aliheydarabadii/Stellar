package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	getmeasurements "api_gateway/internal/measurements/application/get_measurements"
	"api_gateway/internal/measurements/domain"
	gobreaker "github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/suite"
)

type CircuitBreakerSuite struct {
	suite.Suite
}

func TestCircuitBreakerSuite(t *testing.T) {
	suite.Run(t, new(CircuitBreakerSuite))
}

func (s *CircuitBreakerSuite) TestNewCircuitBreakerReaderRejectsNilInnerReader() {
	_, err := NewCircuitBreakerReader(nil, nil)

	s.ErrorIs(err, getmeasurements.ErrMeasurementsReaderRequired)
}

func (s *CircuitBreakerSuite) TestGetMeasurementsDelegatesToInnerReader() {
	reader := &stubMeasurementsReader{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}
	breaker, err := newCircuitBreakerReader(reader, gobreaker.Settings{
		Name: "test-breaker",
	})
	s.Require().NoError(err)

	series, execErr := breaker.GetMeasurements(context.Background(), "asset-1", s.baseTime(), s.baseTime().Add(time.Minute))

	s.Require().NoError(execErr)
	s.Equal("asset-1", series.AssetID)
	s.Equal(1, reader.calls)
}

func (s *CircuitBreakerSuite) TestGetMeasurementsOpensCircuitAfterFailure() {
	reader := &stubMeasurementsReader{
		err: serviceUnavailableError{cause: errors.New("rpc unavailable")},
	}
	breaker, err := newCircuitBreakerReader(reader, gobreaker.Settings{
		Name:        "test-breaker",
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 1 },
		Timeout:     time.Minute,
	})
	s.Require().NoError(err)

	_, firstErr := breaker.GetMeasurements(context.Background(), "asset-1", s.baseTime(), s.baseTime().Add(time.Minute))
	_, secondErr := breaker.GetMeasurements(context.Background(), "asset-1", s.baseTime(), s.baseTime().Add(time.Minute))

	s.Error(firstErr)
	s.Error(secondErr)
	s.Equal(1, reader.calls)
	var unavailable interface {
		error
		MeasurementServiceUnavailable() bool
	}
	s.Require().ErrorAs(secondErr, &unavailable)
	s.True(unavailable.MeasurementServiceUnavailable())
	s.ErrorContains(secondErr, gobreaker.ErrOpenState.Error())
}

func (s *CircuitBreakerSuite) TestGetMeasurementsExcludesInvalidRequestFromBreaker() {
	reader := &stubMeasurementsReader{
		err: getmeasurements.NewDownstreamInvalidRequestError("window too large"),
	}
	breaker, err := newCircuitBreakerReader(reader, gobreaker.Settings{
		Name:        "test-breaker",
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 1 },
		Timeout:     time.Minute,
	})
	s.Require().NoError(err)

	_, firstErr := breaker.GetMeasurements(context.Background(), "asset-1", s.baseTime(), s.baseTime().Add(time.Minute))
	reader.err = nil
	reader.series = domain.MeasurementSeries{AssetID: "asset-1"}
	_, secondErr := breaker.GetMeasurements(context.Background(), "asset-1", s.baseTime(), s.baseTime().Add(time.Minute))

	s.ErrorIs(firstErr, getmeasurements.ErrDownstreamInvalidRequest)
	s.NoError(secondErr)
	s.Equal(2, reader.calls)
}

func (s *CircuitBreakerSuite) baseTime() time.Time {
	return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
}

type stubMeasurementsReader struct {
	calls  int
	series domain.MeasurementSeries
	err    error
}

func (r *stubMeasurementsReader) GetMeasurements(_ context.Context, assetID string, _, _ time.Time) (domain.MeasurementSeries, error) {
	r.calls++
	if r.err != nil {
		return domain.MeasurementSeries{}, r.err
	}

	if r.series.AssetID == "" {
		r.series.AssetID = assetID
	}

	return r.series, nil
}
