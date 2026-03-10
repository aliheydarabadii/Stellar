package query

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGetMeasurementsHandlerHandleRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	handler := NewGetMeasurementsHandler(&fakeMeasurementsReadModel{})
	now := time.Now().UTC()

	testCases := []struct {
		name  string
		query GetMeasurements
		want  error
	}{
		{
			name: "empty asset id",
			query: GetMeasurements{
				AssetID: "",
				From:    now,
				To:      now,
			},
			want: ErrAssetIDRequired,
		},
		{
			name: "from after to",
			query: GetMeasurements{
				AssetID: "asset-1",
				From:    now.Add(time.Second),
				To:      now,
			},
			want: ErrInvalidTimeRange,
		},
		{
			name: "zero timestamps",
			query: GetMeasurements{
				AssetID: "asset-1",
			},
			want: ErrTimestampZero,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := handler.Handle(context.Background(), tc.query)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected error %v, got %v", tc.want, err)
			}
		})
	}
}

func TestGetMeasurementsHandlerHandleReturnsPoints(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	readModel := &fakeMeasurementsReadModel{
		points: []MeasurementPoint{
			{
				Timestamp:   now,
				Setpoint:    10,
				ActivePower: 9.5,
			},
		},
	}
	handler := NewGetMeasurementsHandler(readModel)

	got, err := handler.Handle(context.Background(), GetMeasurements{
		AssetID: " asset-1 ",
		From:    now,
		To:      now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got.AssetID != "asset-1" {
		t.Fatalf("expected trimmed asset id, got %q", got.AssetID)
	}

	if len(got.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(got.Points))
	}

	if readModel.assetID != "asset-1" {
		t.Fatalf("expected read model asset id asset-1, got %q", readModel.assetID)
	}
}

func TestGetMeasurementsHandlerHandleReturnsReadModelError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("read model failed")
	now := time.Now().UTC()
	handler := NewGetMeasurementsHandler(&fakeMeasurementsReadModel{err: wantErr})

	_, err := handler.Handle(context.Background(), GetMeasurements{
		AssetID: "asset-1",
		From:    now,
		To:      now,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}

type fakeMeasurementsReadModel struct {
	assetID string
	from    time.Time
	to      time.Time
	points  []MeasurementPoint
	err     error
}

func (f *fakeMeasurementsReadModel) GetMeasurements(_ context.Context, assetID string, from, to time.Time) ([]MeasurementPoint, error) {
	f.assetID = assetID
	f.from = from
	f.to = to

	if f.err != nil {
		return nil, f.err
	}

	return f.points, nil
}
