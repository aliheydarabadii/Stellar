package app

import "stellar/internal/measurements/app/query"

type Application struct {
	Queries Queries
}

type Queries struct {
	GetMeasurements query.GetMeasurementsHandler
}

func New(readModel query.MeasurementsReadModel) (Application, error) {
	getMeasurements, err := query.NewGetMeasurementsHandler(readModel)
	if err != nil {
		return Application{}, err
	}

	return Application{
		Queries: Queries{
			GetMeasurements: getMeasurements,
		},
	}, nil
}
