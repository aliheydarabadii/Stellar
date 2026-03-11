package app

import (
	"time"

	"stellar/internal/measurements/app/query"
)

type Application struct {
	Queries Queries
}

type Queries struct {
	GetMeasurements query.GetMeasurementsHandler
}

const DefaultMaxQueryRange = query.DefaultMaxQueryRange

type Config struct {
	MaxQueryRange time.Duration
}

func New(readModel query.MeasurementsReadModel) (Application, error) {
	return NewWithConfig(readModel, Config{})
}

func NewWithConfig(readModel query.MeasurementsReadModel, cfg Config) (Application, error) {
	if cfg.MaxQueryRange <= 0 {
		cfg.MaxQueryRange = DefaultMaxQueryRange
	}

	getMeasurements, err := query.NewGetMeasurementsHandlerWithConfig(readModel, query.HandlerConfig{
		MaxQueryRange: cfg.MaxQueryRange,
	})
	if err != nil {
		return Application{}, err
	}

	return Application{
		Queries: Queries{
			GetMeasurements: getMeasurements,
		},
	}, nil
}
