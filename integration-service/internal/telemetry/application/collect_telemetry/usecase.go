// Package collecttelemetry contains the command-side telemetry collection use case.
package collecttelemetry

import (
	"context"
	"errors"

	"stellar/internal/telemetry/domain"
)

var _ CommandHandler = UseCase{}

type UseCase struct {
	assetID    domain.AssetID
	source     TelemetrySource
	repository MeasurementRepository
}

func NewUseCase(assetID domain.AssetID, source TelemetrySource, repository MeasurementRepository) (UseCase, error) {
	switch {
	case assetID == "":
		return UseCase{}, ErrEmptyAssetID
	case source == nil:
		return UseCase{}, ErrNilTelemetrySource
	case repository == nil:
		return UseCase{}, ErrNilMeasurementRepository
	}

	return UseCase{
		assetID:    assetID,
		source:     source,
		repository: repository,
	}, nil
}

func (u UseCase) Handle(ctx context.Context, cmd CollectTelemetry) error {
	reading, err := u.source.Read(ctx)
	if err != nil {
		return errors.Join(ErrTelemetrySource, err)
	}

	measurement, err := domain.NewMeasurement(
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
