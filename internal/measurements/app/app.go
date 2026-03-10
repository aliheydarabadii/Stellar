// Package app wires query handlers for the measurements bounded context.
package app

import "stellar/internal/measurements/app/query"

type Application struct {
	Queries Queries
}

type Queries struct {
	GetMeasurements query.GetMeasurementsHandler
}

func New(readModel query.MeasurementsReadModel) Application {
	return Application{
		Queries: Queries{
			GetMeasurements: query.NewGetMeasurementsHandler(readModel),
		},
	}
}
