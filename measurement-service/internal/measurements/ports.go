package measurements

import (
	"context"
	"time"
)

type MeasurementsReadModel interface {
	GetMeasurements(ctx context.Context, assetID string, from, to time.Time) ([]MeasurementPoint, error)
}
