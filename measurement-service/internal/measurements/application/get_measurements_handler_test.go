package application

import (
	"context"
	"errors"
	"stellar/internal/measurements"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type GetMeasurementsHandlerSuite struct {
	suite.Suite
}

func TestGetMeasurementsHandlerSuite(t *testing.T) {
	suite.Run(t, new(GetMeasurementsHandlerSuite))
}

func (s *GetMeasurementsHandlerSuite) TestNewRejectsNilReadModel() {
	_, err := NewGetMeasurementsHandler(nil)

	s.ErrorIs(err, ErrReadModelUnavailable)
}

func (s *GetMeasurementsHandlerSuite) TestHandleRejectsInvalidInput() {
	handler, err := NewGetMeasurementsHandler(&fakeMeasurementsReadModel{})
	s.Require().NoError(err)

	now := time.Now().UTC()

	testCases := []struct {
		name  string
		query Query
		want  error
	}{
		{
			name: "empty asset id",
			query: Query{
				AssetID: "",
				From:    now,
				To:      now,
			},
			want: ErrAssetIDRequired,
		},
		{
			name: "from after to",
			query: Query{
				AssetID: "asset-1",
				From:    now.Add(time.Second),
				To:      now,
			},
			want: ErrInvalidTimeRange,
		},
		{
			name: "zero timestamps",
			query: Query{
				AssetID: "asset-1",
			},
			want: ErrTimestampZero,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			_, err := handler.Handle(context.Background(), tc.query)
			s.ErrorIs(err, tc.want)
		})
	}
}

func (s *GetMeasurementsHandlerSuite) TestHandleRejectsRangeLargerThanConfiguredLimit() {
	handler, err := NewGetMeasurementsHandlerWithConfig(&fakeMeasurementsReadModel{}, Config{
		MaxQueryRange: 5 * time.Minute,
	})
	s.Require().NoError(err)

	now := time.Now().UTC().Truncate(time.Second)

	_, err = handler.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    now,
		To:      now.Add(5*time.Minute + time.Second),
	})

	s.ErrorIs(err, ErrQueryRangeTooLarge)
}

func (s *GetMeasurementsHandlerSuite) TestHandleAllowsRangeAtConfiguredLimit() {
	readModel := &fakeMeasurementsReadModel{}
	handler, err := NewGetMeasurementsHandlerWithConfig(readModel, Config{
		MaxQueryRange: 5 * time.Minute,
	})
	s.Require().NoError(err)

	now := time.Now().UTC().Truncate(time.Second)

	_, err = handler.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    now,
		To:      now.Add(5 * time.Minute),
	})

	s.NoError(err)
}

func (s *GetMeasurementsHandlerSuite) TestHandleReturnsPoints() {
	now := time.Now().In(time.FixedZone("offset", 2*60*60)).Truncate(time.Second)
	readModel := &fakeMeasurementsReadModel{
		points: []measurements.MeasurementPoint{
			{
				Timestamp:   now.UTC(),
				Setpoint:    10,
				ActivePower: 9.5,
			},
		},
	}

	handler, err := NewGetMeasurementsHandler(readModel)
	s.Require().NoError(err)

	got, err := handler.Handle(context.Background(), Query{
		AssetID: " asset-1 ",
		From:    now,
		To:      now.Add(time.Second),
	})
	s.Require().NoError(err)

	s.Equal("asset-1", got.AssetID)
	s.Len(got.Points, 1)
	s.Equal("asset-1", readModel.assetID)
	s.Equal(time.UTC, readModel.from.Location())
	s.Equal(time.UTC, readModel.to.Location())
	s.True(readModel.from.Equal(now.UTC()))
	s.True(readModel.to.Equal(now.Add(time.Second).UTC()))
}

func (s *GetMeasurementsHandlerSuite) TestHandleReturnsReadModelError() {
	wantErr := errors.New("read model failed")
	now := time.Now().UTC()

	handler, err := NewGetMeasurementsHandler(&fakeMeasurementsReadModel{err: wantErr})
	s.Require().NoError(err)

	_, err = handler.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    now,
		To:      now,
	})

	s.ErrorIs(err, wantErr)
}

type fakeMeasurementsReadModel struct {
	assetID string
	from    time.Time
	to      time.Time
	points  []measurements.MeasurementPoint
	err     error
}

func (f *fakeMeasurementsReadModel) GetMeasurements(_ context.Context, assetID string, from, to time.Time) ([]measurements.MeasurementPoint, error) {
	f.assetID = assetID
	f.from = from
	f.to = to

	if f.err != nil {
		return nil, f.err
	}

	return f.points, nil
}
