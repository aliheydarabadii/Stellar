package query

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrAssetIDRequired      = errors.New("asset id is required")
	ErrInvalidTimeRange     = errors.New("from must not be after to")
	ErrTimestampZero        = errors.New("from and to must be set")
	ErrReadModelUnavailable = errors.New("measurements read model unavailable")
)

type GetMeasurements struct {
	AssetID string
	From    time.Time
	To      time.Time
}

type MeasurementsReadModel interface {
	GetMeasurements(ctx context.Context, assetID string, from, to time.Time) ([]MeasurementPoint, error)
}

type GetMeasurementsHandler struct {
	readModel MeasurementsReadModel
}

func NewGetMeasurementsHandler(readModel MeasurementsReadModel) GetMeasurementsHandler {
	return GetMeasurementsHandler{readModel: readModel}
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
