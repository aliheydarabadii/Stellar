package application

import (
	"context"
	"fmt"
	"stellar/internal/measurements"
	"strings"
	"time"
)

type GetMeasurementsHandler struct {
	readModel     measurements.MeasurementsReadModel
	maxQueryRange time.Duration
}

func NewGetMeasurementsHandler(readModel measurements.MeasurementsReadModel) (GetMeasurementsHandler, error) {
	return NewGetMeasurementsHandlerWithConfig(readModel, Config{})
}

type Config struct {
	MaxQueryRange time.Duration
}

func NewGetMeasurementsHandlerWithConfig(readModel measurements.MeasurementsReadModel, cfg Config) (GetMeasurementsHandler, error) {
	if readModel == nil {
		return GetMeasurementsHandler{}, ErrReadModelUnavailable
	}
	if cfg.MaxQueryRange <= 0 {
		cfg.MaxQueryRange = DefaultMaxQueryRange
	}

	return GetMeasurementsHandler{
		readModel:     readModel,
		maxQueryRange: cfg.MaxQueryRange,
	}, nil
}

func (h GetMeasurementsHandler) Handle(ctx context.Context, qry Query) (measurements.MeasurementSeries, error) {
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
	case h.maxQueryRange > 0 && qry.To.Sub(qry.From) > h.maxQueryRange:
		return measurements.MeasurementSeries{}, fmt.Errorf("%w: max query range is %s", ErrQueryRangeTooLarge, h.maxQueryRange)
	case h.readModel == nil:
		return measurements.MeasurementSeries{}, ErrReadModelUnavailable
	}

	points, err := h.readModel.GetMeasurements(ctx, assetID, from, to)
	if err != nil {
		return measurements.MeasurementSeries{}, err
	}

	return measurements.MeasurementSeries{
		AssetID: assetID,
		Points:  points,
	}, nil
}
