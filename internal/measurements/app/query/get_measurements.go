package query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrAssetIDRequired      = errors.New("asset id is required")
	ErrInvalidTimeRange     = errors.New("from must not be after to")
	ErrQueryRangeTooLarge   = errors.New("query time range exceeds maximum allowed window")
	ErrTimestampZero        = errors.New("from and to must be set")
	ErrReadModelUnavailable = errors.New("measurements read model unavailable")
)

const DefaultMaxQueryRange = 15 * time.Minute

type GetMeasurements struct {
	AssetID string
	From    time.Time
	To      time.Time
}

type MeasurementsReadModel interface {
	GetMeasurements(ctx context.Context, assetID string, from, to time.Time) ([]MeasurementPoint, error)
}

type GetMeasurementsHandler struct {
	readModel     MeasurementsReadModel
	maxQueryRange time.Duration
}

func NewGetMeasurementsHandler(readModel MeasurementsReadModel) (GetMeasurementsHandler, error) {
	return NewGetMeasurementsHandlerWithConfig(readModel, HandlerConfig{})
}

type HandlerConfig struct {
	MaxQueryRange time.Duration
}

func NewGetMeasurementsHandlerWithConfig(readModel MeasurementsReadModel, cfg HandlerConfig) (GetMeasurementsHandler, error) {
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

func (h GetMeasurementsHandler) Handle(ctx context.Context, qry GetMeasurements) (MeasurementSeries, error) {
	assetID := strings.TrimSpace(qry.AssetID)

	switch {
	case assetID == "":
		return MeasurementSeries{}, ErrAssetIDRequired
	case qry.From.IsZero() || qry.To.IsZero():
		return MeasurementSeries{}, ErrTimestampZero
	case qry.From.After(qry.To):
		return MeasurementSeries{}, ErrInvalidTimeRange
	case h.maxQueryRange > 0 && qry.To.Sub(qry.From) > h.maxQueryRange:
		return MeasurementSeries{}, fmt.Errorf("%w: max query range is %s", ErrQueryRangeTooLarge, h.maxQueryRange)
	case h.readModel == nil:
		return MeasurementSeries{}, ErrReadModelUnavailable
	}

	points, err := h.readModel.GetMeasurements(ctx, assetID, qry.From, qry.To)
	if err != nil {
		return MeasurementSeries{}, err
	}

	return MeasurementSeries{
		AssetID: assetID,
		Points:  points,
	}, nil
}
