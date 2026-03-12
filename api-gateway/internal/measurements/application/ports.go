package application

import (
	"api_gateway/internal/measurements"
	"context"
	"time"
)

type QueryHandler interface {
	Handle(ctx context.Context, qry Query) (measurements.MeasurementSeries, error)
}

type MeasurementsCache interface {
	Get(ctx context.Context, key string) (measurements.MeasurementSeries, bool, error)
	Set(ctx context.Context, key string, value measurements.MeasurementSeries, ttl time.Duration) error
}

type CacheFailureObserver interface {
	CacheOperationFailed(ctx context.Context, operation, key string, err error)
}

type CacheKeyBuilder func(assetID string, from, to time.Time) string
