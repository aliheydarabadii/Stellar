package collecttelemetry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	collecttelemetry "stellar/internal/telemetry/application/collect_telemetry"
	collecttelemetrymocks "stellar/internal/telemetry/application/collect_telemetry/mocks"
	"stellar/internal/telemetry/domain"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type CollectTelemetryHandlerSuite struct {
	suite.Suite
	ctx         context.Context
	collectedAt time.Time
	source      *collecttelemetrymocks.TelemetrySource
	repository  *collecttelemetrymocks.MeasurementRepository
	handler     collecttelemetry.UseCase
}

func TestCollectTelemetryHandlerSuite(t *testing.T) {
	t.Parallel()

	suite.Run(t, new(CollectTelemetryHandlerSuite))
}

func (s *CollectTelemetryHandlerSuite) SetupTest() {
	s.ctx = context.Background()
	s.collectedAt = time.Date(2026, time.March, 9, 12, 0, 0, 0, time.UTC)
	s.source = collecttelemetrymocks.NewTelemetrySource(s.T())
	s.repository = collecttelemetrymocks.NewMeasurementRepository(s.T())
	var err error
	s.handler, err = collecttelemetry.NewUseCase(domain.DefaultAssetID, s.source, s.repository)
	s.Require().NoError(err)
}

func (s *CollectTelemetryHandlerSuite) TestValidReadingGetsSaved() {
	reading := collecttelemetry.TelemetryReading{
		Setpoint:    100,
		ActivePower: 80,
	}
	expectedMeasurement := domain.Measurement{
		AssetID:     domain.DefaultAssetID,
		Setpoint:    100,
		ActivePower: 80,
		CollectedAt: s.collectedAt,
	}

	s.source.EXPECT().Read(mock.Anything).Return(reading, nil).Once()
	s.repository.EXPECT().Save(mock.Anything, expectedMeasurement).Return(nil).Once()

	err := s.handler.Handle(s.ctx, collecttelemetry.CollectTelemetry{CollectedAt: s.collectedAt})

	s.NoError(err)
}

func (s *CollectTelemetryHandlerSuite) TestInvalidReadingDoesNotGetSaved() {
	s.source.EXPECT().Read(mock.Anything).Return(collecttelemetry.TelemetryReading{
		Setpoint:    10,
		ActivePower: 20,
	}, nil).Once()

	err := s.handler.Handle(s.ctx, collecttelemetry.CollectTelemetry{CollectedAt: s.collectedAt})

	s.Error(err)
	s.ErrorIs(err, collecttelemetry.ErrInvalidTelemetry)
	s.ErrorIs(err, domain.ErrInvalidMeasurement)
	s.repository.AssertNotCalled(s.T(), "Save", mock.Anything, mock.Anything)
}

func (s *CollectTelemetryHandlerSuite) TestSourceErrorIsReturned() {
	sourceErr := errors.New("source unavailable")

	s.source.EXPECT().Read(mock.Anything).Return(collecttelemetry.TelemetryReading{}, sourceErr).Once()

	err := s.handler.Handle(s.ctx, collecttelemetry.CollectTelemetry{CollectedAt: s.collectedAt})

	s.Error(err)
	s.ErrorIs(err, collecttelemetry.ErrTelemetrySource)
	s.ErrorIs(err, sourceErr)
	s.repository.AssertNotCalled(s.T(), "Save", mock.Anything, mock.Anything)
}

func (s *CollectTelemetryHandlerSuite) TestRepositoryErrorIsReturned() {
	repositoryErr := errors.New("repository unavailable")
	expectedMeasurement := domain.Measurement{
		AssetID:     domain.DefaultAssetID,
		Setpoint:    100,
		ActivePower: 80,
		CollectedAt: s.collectedAt,
	}

	s.source.EXPECT().Read(mock.Anything).Return(collecttelemetry.TelemetryReading{
		Setpoint:    100,
		ActivePower: 80,
	}, nil).Once()
	s.repository.EXPECT().Save(mock.Anything, expectedMeasurement).Return(repositoryErr).Once()

	err := s.handler.Handle(s.ctx, collecttelemetry.CollectTelemetry{CollectedAt: s.collectedAt})

	s.Error(err)
	s.ErrorIs(err, collecttelemetry.ErrMeasurementPersistence)
	s.ErrorIs(err, repositoryErr)
}

func (s *CollectTelemetryHandlerSuite) TestNewCollectTelemetryHandlerRejectsInvalidArguments() {
	testCases := []struct {
		name       string
		assetID    domain.AssetID
		source     collecttelemetry.TelemetrySource
		repository collecttelemetry.MeasurementRepository
		wantErr    error
	}{
		{
			name:       "empty asset id",
			assetID:    "",
			source:     s.source,
			repository: s.repository,
			wantErr:    collecttelemetry.ErrEmptyAssetID,
		},
		{
			name:       "nil source",
			assetID:    domain.DefaultAssetID,
			source:     nil,
			repository: s.repository,
			wantErr:    collecttelemetry.ErrNilTelemetrySource,
		},
		{
			name:       "nil repository",
			assetID:    domain.DefaultAssetID,
			source:     s.source,
			repository: nil,
			wantErr:    collecttelemetry.ErrNilMeasurementRepository,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			handler, err := collecttelemetry.NewUseCase(tc.assetID, tc.source, tc.repository)

			s.Error(err)
			s.ErrorIs(err, tc.wantErr)
			s.Equal(collecttelemetry.UseCase{}, handler)
		})
	}
}
