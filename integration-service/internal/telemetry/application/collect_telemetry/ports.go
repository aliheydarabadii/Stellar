package collecttelemetry

import (
	"context"

	"stellar/internal/telemetry/domain"
)

type TelemetrySource interface {
	Read(ctx context.Context) (TelemetryReading, error)
}

type MeasurementRepository interface {
	Save(ctx context.Context, measurement domain.Measurement) error
}

type CommandHandler interface {
	Handle(ctx context.Context, cmd CollectTelemetry) error
}
