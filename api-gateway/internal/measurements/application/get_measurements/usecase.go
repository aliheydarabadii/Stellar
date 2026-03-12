package getmeasurements

import (
	"context"
	"strings"

	"api_gateway/internal/measurements/domain"
)

type UseCase struct {
	reader MeasurementsReader
}

func NewUseCase(reader MeasurementsReader) (UseCase, error) {
	if reader == nil {
		return UseCase{}, ErrMeasurementsReaderRequired
	}

	return UseCase{
		reader: reader,
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
	}

	series, err := u.reader.GetMeasurements(ctx, assetID, from, to)
	if err != nil {
		return domain.MeasurementSeries{}, mapMeasurementsReaderError(err)
	}

	return series, nil
}
