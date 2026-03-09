package app

import "stellar/internal/telemetry/app/command"

type Application struct {
	Commands Commands
}

type Commands struct {
	CollectTelemetry command.CollectTelemetryHandler
}

func NewApplication(source command.TelemetrySource, repository command.MeasurementRepository) Application {
	return Application{
		Commands: Commands{
			CollectTelemetry: command.NewCollectTelemetryHandler(source, repository),
		},
	}
}
