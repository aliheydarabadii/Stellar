// Package app wires the telemetry command-side application services.
package app

import (
	"stellar/internal/telemetry/app/command"
	"stellar/internal/telemetry/domain"
)

type Application struct {
	Commands Commands
}

type Commands struct {
	CollectTelemetry command.CollectTelemetryHandler
}

func NewApplication(assetID domain.AssetID, source command.TelemetrySource, repository command.MeasurementRepository) (Application, error) {
	collectTelemetry, err := command.NewCollectTelemetryHandler(assetID, source, repository)
	if err != nil {
		return Application{}, err
	}

	return Application{
		Commands: Commands{
			CollectTelemetry: collectTelemetry,
		},
	}, nil
}
