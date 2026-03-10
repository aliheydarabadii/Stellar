// Package command contains command-side application use cases for telemetry.
package command

import (
	"context"
	"errors"
	"time"

	"stellar/internal/telemetry/domain"
)

var ErrInvalidTelemetry = errors.New("invalid telemetry")

var ErrTelemetrySource = errors.New("telemetry source")

var ErrMeasurementPersistence = errors.New("measurement persistence")

type CollectTelemetry struct {
	CollectedAt time.Time
}

type TelemetryReading struct {
	Setpoint    float64
	ActivePower float64
}

type TelemetrySource interface {
	Read(ctx context.Context) (TelemetryReading, error)
}

type MeasurementRepository interface {
	Save(ctx context.Context, measurement domain.Measurement) error
}

type CollectTelemetryHandler struct {
	assetID    domain.AssetID
	source     TelemetrySource
	repository MeasurementRepository
}

func NewCollectTelemetryHandler(assetID domain.AssetID, source TelemetrySource, repository MeasurementRepository) CollectTelemetryHandler {
	return CollectTelemetryHandler{
		assetID:    assetID,
		source:     source,
		repository: repository,
	}
}

func (h CollectTelemetryHandler) Handle(ctx context.Context, cmd CollectTelemetry) error {
	reading, err := h.source.Read(ctx)
	if err != nil {
		return errors.Join(ErrTelemetrySource, err)
	}

	measurement, err := domain.NewMeasurement(
		h.assetID,
		reading.Setpoint,
		reading.ActivePower,
		cmd.CollectedAt,
	)
	if err != nil {
		return errors.Join(ErrInvalidTelemetry, err)
	}

	if err := h.repository.Save(ctx, measurement); err != nil {
		return errors.Join(ErrMeasurementPersistence, err)
	}

	return nil
}
