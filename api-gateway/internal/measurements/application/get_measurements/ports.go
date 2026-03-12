package getmeasurements

import (
	"context"
	"time"

	"api_gateway/internal/measurements/domain"
)

type MeasurementsClient interface {
	GetMeasurements(ctx context.Context, assetID string, from, to time.Time) (domain.MeasurementSeries, error)
}

type QueryHandler interface {
	Handle(ctx context.Context, qry Query) (Result, error)
}

type MeasurementsCache interface {
	Get(ctx context.Context, key string) (domain.MeasurementSeries, bool, error)
	Set(ctx context.Context, key string, value domain.MeasurementSeries, ttl time.Duration) error
}

type CacheFailureObserver interface {
	CacheOperationFailed(ctx context.Context, operation, key string, err error)
}

type CacheKeyBuilder func(assetID string, from, to time.Time) string
