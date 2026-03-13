package application

import (
	"context"
	"errors"
	"stellar/internal/measurements"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type UseCaseSuite struct {
	suite.Suite
}

func TestUseCaseSuite(t *testing.T) {
	suite.Run(t, new(UseCaseSuite))
}

func (s *UseCaseSuite) TestNewRejectsNilReadModel() {
	_, err := NewUseCase(nil)

	s.ErrorIs(err, ErrReadModelUnavailable)
}

func (s *UseCaseSuite) TestHandleRejectsInvalidInput() {
	useCase, err := NewUseCase(&fakeMeasurementsReadModel{})
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
			_, err := useCase.Handle(context.Background(), tc.query)
			s.ErrorIs(err, tc.want)
		})
	}
}

func (s *UseCaseSuite) TestHandleRejectsRangeLargerThanConfiguredLimit() {
	useCase, err := NewUseCaseWithConfig(&fakeMeasurementsReadModel{}, Config{
		MaxQueryRange: 5 * time.Minute,
	})
	s.Require().NoError(err)

	now := time.Now().UTC().Truncate(time.Second)

	_, err = useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    now,
		To:      now.Add(5*time.Minute + time.Second),
	})

	s.ErrorIs(err, ErrQueryRangeTooLarge)
}

func (s *UseCaseSuite) TestHandleAllowsRangeAtConfiguredLimit() {
	readModel := &fakeMeasurementsReadModel{}
	useCase, err := NewUseCaseWithConfig(readModel, Config{
		MaxQueryRange: 5 * time.Minute,
	})
	s.Require().NoError(err)

	now := time.Now().UTC().Truncate(time.Second)

	_, err = useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    now,
		To:      now.Add(5 * time.Minute),
	})

	s.NoError(err)
}

func (s *UseCaseSuite) TestHandleReturnsPoints() {
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

	useCase, err := NewUseCase(readModel)
	s.Require().NoError(err)

	got, err := useCase.Handle(context.Background(), Query{
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

func (s *UseCaseSuite) TestHandleReturnsReadModelError() {
	wantErr := errors.New("read model failed")
	now := time.Now().UTC()

	useCase, err := NewUseCase(&fakeMeasurementsReadModel{err: wantErr})
	s.Require().NoError(err)

	_, err = useCase.Handle(context.Background(), Query{
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
