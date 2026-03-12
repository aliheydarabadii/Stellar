package getmeasurements

import (
	"context"
	"fmt"
	"strings"
	"time"

	"stellar/internal/measurements/domain"
)

var _ QueryHandler = UseCase{}

type UseCase struct {
	readModel     MeasurementsReadModel
	maxQueryRange time.Duration
}

func NewUseCase(readModel MeasurementsReadModel) (UseCase, error) {
	return NewUseCaseWithConfig(readModel, Config{})
}

type Config struct {
	MaxQueryRange time.Duration
}

func NewUseCaseWithConfig(readModel MeasurementsReadModel, cfg Config) (UseCase, error) {
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

func (u UseCase) Handle(ctx context.Context, qry Query) (domain.MeasurementSeries, error) {
	assetID := strings.TrimSpace(qry.AssetID)
	from := qry.From.UTC()
	to := qry.To.UTC()

	switch {
	case assetID == "":
		return domain.MeasurementSeries{}, ErrAssetIDRequired
	case qry.From.IsZero() || qry.To.IsZero():
		return domain.MeasurementSeries{}, ErrTimestampZero
	case qry.From.After(qry.To):
		return domain.MeasurementSeries{}, ErrInvalidTimeRange
	case u.maxQueryRange > 0 && qry.To.Sub(qry.From) > u.maxQueryRange:
		return domain.MeasurementSeries{}, fmt.Errorf("%w: max query range is %s", ErrQueryRangeTooLarge, u.maxQueryRange)
	case u.readModel == nil:
		return domain.MeasurementSeries{}, ErrReadModelUnavailable
	}

	points, err := u.readModel.GetMeasurements(ctx, assetID, from, to)
	if err != nil {
		return domain.MeasurementSeries{}, err
	}

	return domain.MeasurementSeries{
		AssetID: assetID,
		Points:  points,
	}, nil
}
