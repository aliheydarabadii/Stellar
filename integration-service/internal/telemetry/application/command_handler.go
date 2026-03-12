// Package application contains the command-side telemetry collection use case.
package application

import (
	"context"
	"errors"

	telemetry "stellar/internal/telemetry"
)

type CollectTelemetryHandler struct {
	assetID    telemetry.AssetID
	source     telemetry.TelemetrySource
	repository telemetry.MeasurementRepository
}

func NewCollectTelemetryHandler(assetID telemetry.AssetID, source telemetry.TelemetrySource, repository telemetry.MeasurementRepository) (CollectTelemetryHandler, error) {
	switch {
	case assetID == "":
		return CollectTelemetryHandler{}, ErrEmptyAssetID
	case source == nil:
		return CollectTelemetryHandler{}, ErrNilTelemetrySource
	case repository == nil:
		return CollectTelemetryHandler{}, ErrNilMeasurementRepository
	}

	return CollectTelemetryHandler{
		assetID:    assetID,
		source:     source,
		repository: repository,
	}, nil
}

func (u CollectTelemetryHandler) Handle(ctx context.Context, cmd CollectTelemetry) error {
	reading, err := u.source.Read(ctx)
	if err != nil {
		return errors.Join(ErrTelemetrySource, err)
	}

	measurement, err := telemetry.NewMeasurement(
		u.assetID,
		reading.Setpoint,
		reading.ActivePower,
		cmd.CollectedAt,
	)
	if err != nil {
		return errors.Join(ErrInvalidTelemetry, err)
	}

	if err := u.repository.Save(ctx, measurement); err != nil {
		return errors.Join(ErrMeasurementPersistence, err)
	}

	return nil
}
