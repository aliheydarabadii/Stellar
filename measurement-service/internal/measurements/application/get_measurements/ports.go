package getmeasurements

import (
	"context"
	"time"

	"stellar/internal/measurements/domain"
)

type MeasurementsReadModel interface {
	GetMeasurements(ctx context.Context, assetID string, from, to time.Time) ([]domain.MeasurementPoint, error)
}

type QueryHandler interface {
	Handle(ctx context.Context, qry Query) (domain.MeasurementSeries, error)
}
