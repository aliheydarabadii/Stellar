package application

import (
	"context"
	"strings"

	"api_gateway/internal/measurements"
)

type GetMeasurementsHandler struct {
	reader measurements.MeasurementsReader
}

func NewMeasurementsHandler(reader measurements.MeasurementsReader) (GetMeasurementsHandler, error) {
	if reader == nil {
		return GetMeasurementsHandler{}, ErrMeasurementsReaderRequired
	}

	return GetMeasurementsHandler{
		reader: reader,
	}, nil
}

func (u GetMeasurementsHandler) Handle(ctx context.Context, qry Query) (measurements.MeasurementSeries, error) {
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
	}

	series, err := u.reader.GetMeasurements(ctx, assetID, from, to)
	if err != nil {
		return measurements.MeasurementSeries{}, mapMeasurementsReaderError(err)
	}

	return series, nil
}
