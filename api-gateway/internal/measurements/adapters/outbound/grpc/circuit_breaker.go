package grpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"api_gateway/internal/measurements"
	getmeasurements "api_gateway/internal/measurements/application"

	gobreaker "github.com/sony/gobreaker/v2"
)

const (
	defaultCircuitBreakerName     = "measurement-service-reader"
	defaultCircuitBreakerInterval = 30 * time.Second
	defaultCircuitBreakerTimeout  = 15 * time.Second
)

type CircuitBreakerReader struct {
	inner   measurements.MeasurementsReader
	breaker *gobreaker.CircuitBreaker[measurements.MeasurementSeries]
}

func NewCircuitBreakerReader(inner measurements.MeasurementsReader, logger *slog.Logger) (*CircuitBreakerReader, error) {
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

func newCircuitBreakerReader(inner measurements.MeasurementsReader, settings gobreaker.Settings) (*CircuitBreakerReader, error) {
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
		breaker: gobreaker.NewCircuitBreaker[measurements.MeasurementSeries](settings),
	}, nil
}

func (r *CircuitBreakerReader) GetMeasurements(ctx context.Context, assetID string, from, to time.Time) (measurements.MeasurementSeries, error) {
	series, err := r.breaker.Execute(func() (measurements.MeasurementSeries, error) {
		return r.inner.GetMeasurements(ctx, assetID, from, to)
	})
	if err != nil {
		return measurements.MeasurementSeries{}, mapCircuitBreakerError(err)
	}

	return series, nil
}

func mapCircuitBreakerError(err error) error {
	switch {
	case errors.Is(err, gobreaker.ErrOpenState), errors.Is(err, gobreaker.ErrTooManyRequests):
		return fmt.Errorf("%w: %w", measurements.ErrMeasurementsReaderUnavailable, err)
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

	return errors.Is(err, measurements.ErrMeasurementsReaderInvalidRequest)
}
