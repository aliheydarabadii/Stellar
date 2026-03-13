package measurements

import (
	"context"
	"stellar/internal/measurements/application"
	"time"
)

type MeasurementsReadModel interface {
	GetMeasurements(ctx context.Context, assetID string, from, to time.Time) ([]MeasurementPoint, error)
}

type QueryHandler interface {
	Handle(ctx context.Context, qry application.getmeasurements) (MeasurementSeries, error)
}
