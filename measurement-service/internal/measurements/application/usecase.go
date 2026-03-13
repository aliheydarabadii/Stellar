package application

import (
	"context"
	"fmt"
	"stellar/internal/measurements"
	"strings"
	"time"
)

var _ measurements.QueryHandler = UseCase{}

type UseCase struct {
	readModel     measurements.MeasurementsReadModel
	maxQueryRange time.Duration
}

func NewUseCase(readModel measurements.MeasurementsReadModel) (UseCase, error) {
	return NewUseCaseWithConfig(readModel, Config{})
}

type Config struct {
	MaxQueryRange time.Duration
}

func NewUseCaseWithConfig(readModel measurements.MeasurementsReadModel, cfg Config) (UseCase, error) {
	if readModel == nil {
		return UseCase{}, ErrReadModelUnavailable
	}
	if cfg.MaxQueryRange <= 0 {
		cfg.MaxQueryRange = DefaultMaxQueryRange
	}

	return UseCase{
		readModel:     readModel,
		maxQueryRange: cfg.MaxQueryRange,
	}, nil
}

func (u UseCase) Handle(ctx context.Context, qry Query) (measurements.MeasurementSeries, error) {
	assetID := strings.TrimSpace(qry.AssetID)
	from := qry.From.UTC()
	to := qry.To.UTC()

	switch {
	case assetID == "":
		return measurements.MeasurementSeries{}, ErrAssetIDRequired
	case qry.From.IsZero() || qry.To.IsZero():
		return measurements.MeasurementSeries{}, ErrTimestampZero
	case qry.From.After(qry.To):
		return measurements.MeasurementSeries{}, ErrInvalidTimeRange
	case u.maxQueryRange > 0 && qry.To.Sub(qry.From) > u.maxQueryRange:
		return measurements.MeasurementSeries{}, fmt.Errorf("%w: max query range is %s", ErrQueryRangeTooLarge, u.maxQueryRange)
	case u.readModel == nil:
		return measurements.MeasurementSeries{}, ErrReadModelUnavailable
	}

	points, err := u.readModel.GetMeasurements(ctx, assetID, from, to)
	if err != nil {
		return measurements.MeasurementSeries{}, err
	}

	return measurements.MeasurementSeries{
		AssetID: assetID,
		Points:  points,
	}, nil
}
