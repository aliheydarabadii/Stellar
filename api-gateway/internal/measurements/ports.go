package measurements

import (
	"context"
	"time"
)

type MeasurementsReader interface {
	GetMeasurements(ctx context.Context, assetID string, from, to time.Time) (MeasurementSeries, error)
}
