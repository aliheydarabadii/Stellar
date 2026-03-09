package command

import (
	"context"
	"errors"
	"time"

	"stellar/internal/telemetry/domain"
)

var ErrInvalidTelemetry = errors.New("invalid telemetry")

type CollectTelemetry struct {
	CollectedAt time.Time
}

type TelemetryReading struct {
	AssetID     domain.AssetID
	Setpoint    float64
	ActivePower float64
}

type TelemetrySource interface {
	Read(ctx context.Context, collectedAt time.Time) (TelemetryReading, error)
}

type MeasurementRepository interface {
	Save(ctx context.Context, measurement domain.Measurement) error
}

type CollectTelemetryHandler struct {
	source     TelemetrySource
	repository MeasurementRepository
}

func NewCollectTelemetryHandler(source TelemetrySource, repository MeasurementRepository) CollectTelemetryHandler {
	return CollectTelemetryHandler{
		source:     source,
		repository: repository,
	}
}

func (h CollectTelemetryHandler) Handle(ctx context.Context, cmd CollectTelemetry) error {
	reading, err := h.source.Read(ctx, cmd.CollectedAt)
	if err != nil {
		return err
	}

	measurement, err := domain.NewMeasurement(
		reading.AssetID,
		reading.Setpoint,
		reading.ActivePower,
		cmd.CollectedAt,
	)
	if err != nil {
		return errors.Join(ErrInvalidTelemetry, err)
	}

	if err := h.repository.Save(ctx, measurement); err != nil {
		return err
	}

	return nil
}
