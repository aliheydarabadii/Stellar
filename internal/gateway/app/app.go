package app

import (
	"log/slog"
	"time"

	"api_gateway/internal/gateway/app/query"
)

type Application struct {
	Queries Queries
}

type Queries struct {
	GetMeasurements query.GetMeasurementsHandler
}

func New(
	measurementsClient query.MeasurementsClient,
	measurementsCache query.MeasurementsCache,
	cacheTTL time.Duration,
	buildCacheKey query.CacheKeyBuilder,
	logger *slog.Logger,
) (Application, error) {
	getMeasurements, err := query.NewGetMeasurementsHandler(measurementsClient, measurementsCache, cacheTTL, buildCacheKey, logger)
	if err != nil {
		return Application{}, err
	}

	return Application{
		Queries: Queries{
			GetMeasurements: getMeasurements,
		},
	}, nil
}
