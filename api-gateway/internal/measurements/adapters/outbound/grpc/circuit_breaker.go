package grpc

import (
	"context"
	"errors"
	"log/slog"
	"time"

	getmeasurements "api_gateway/internal/measurements/application/get_measurements"
	"api_gateway/internal/measurements/domain"
	gobreaker "github.com/sony/gobreaker/v2"
)

const (
	defaultCircuitBreakerName     = "measurement-service-reader"
	defaultCircuitBreakerInterval = 30 * time.Second
	defaultCircuitBreakerTimeout  = 15 * time.Second
)

type CircuitBreakerReader struct {
	inner   getmeasurements.MeasurementsReader
	breaker *gobreaker.CircuitBreaker[domain.MeasurementSeries]
}

func NewCircuitBreakerReader(inner getmeasurements.MeasurementsReader, logger *slog.Logger) (*CircuitBreakerReader, error) {
	settings := gobreaker.Settings{
		Name:        defaultCircuitBreakerName,
		MaxRequests: 1,
		Interval:    defaultCircuitBreakerInterval,
		Timeout:     defaultCircuitBreakerTimeout,
		OnStateChange: func(name string, from, to gobreaker.State) {
			if logger == nil {
				return
			}

			logger.Warn("circuit breaker state changed", "name", name, "from", from.String(), "to", to.String())
		},
		IsExcluded: shouldExcludeFromCircuitBreaker,
	}

	return newCircuitBreakerReader(inner, settings)
}

func newCircuitBreakerReader(inner getmeasurements.MeasurementsReader, settings gobreaker.Settings) (*CircuitBreakerReader, error) {
	if inner == nil {
		return nil, getmeasurements.ErrMeasurementsReaderRequired
	}
	if settings.Name == "" {
		settings.Name = defaultCircuitBreakerName
	}
	if settings.IsExcluded == nil {
		settings.IsExcluded = shouldExcludeFromCircuitBreaker
	}

	return &CircuitBreakerReader{
		inner:   inner,
		breaker: gobreaker.NewCircuitBreaker[domain.MeasurementSeries](settings),
	}, nil
}

func (r *CircuitBreakerReader) GetMeasurements(ctx context.Context, assetID string, from, to time.Time) (domain.MeasurementSeries, error) {
	series, err := r.breaker.Execute(func() (domain.MeasurementSeries, error) {
		return r.inner.GetMeasurements(ctx, assetID, from, to)
	})
	if err != nil {
		return domain.MeasurementSeries{}, mapCircuitBreakerError(err)
	}

	return series, nil
}

func mapCircuitBreakerError(err error) error {
	switch {
	case errors.Is(err, gobreaker.ErrOpenState), errors.Is(err, gobreaker.ErrTooManyRequests):
		return serviceUnavailableError{cause: err}
	default:
		return err
	}
}

func shouldExcludeFromCircuitBreaker(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}

	var invalidRequest interface {
		DownstreamInvalidRequestMessage() string
	}
	return errors.As(err, &invalidRequest)
}
