package telemetry

import "context"

type TelemetryReading struct {
	Setpoint    float64
	ActivePower float64
}

type TelemetrySource interface {
	Read(ctx context.Context) (TelemetryReading, error)
}

type MeasurementRepository interface {
	Save(ctx context.Context, measurement Measurement) error
}
