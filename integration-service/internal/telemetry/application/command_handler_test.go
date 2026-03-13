package application_test

import (
	"context"
	"errors"
	"stellar/internal/telemetry/adapters/mocks"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	telemetry "stellar/internal/telemetry"
	collecttelemetry "stellar/internal/telemetry/application"
)

type CollectTelemetryHandlerSuite struct {
	suite.Suite
	ctx         context.Context
	collectedAt time.Time
	source      *mocks.TelemetrySource
	repository  *mocks.MeasurementRepository
	handler     collecttelemetry.CollectTelemetryHandler
}

func TestCollectTelemetryHandlerSuite(t *testing.T) {
	t.Parallel()

	suite.Run(t, new(CollectTelemetryHandlerSuite))
}

func (s *CollectTelemetryHandlerSuite) SetupTest() {
	s.ctx = context.Background()
	s.collectedAt = time.Date(2026, time.March, 9, 12, 0, 0, 0, time.UTC)
	s.source = mocks.NewTelemetrySource(s.T())
	s.repository = mocks.NewMeasurementRepository(s.T())
	var err error
	s.handler, err = collecttelemetry.NewCollectTelemetryHandler(telemetry.DefaultAssetID, s.source, s.repository)
	s.Require().NoError(err)
}

func (s *CollectTelemetryHandlerSuite) TestValidReadingGetsSaved() {
	reading := telemetry.TelemetryReading{
		Setpoint:    100,
		ActivePower: 80,
	}
	expectedMeasurement := telemetry.Measurement{
		AssetID:     telemetry.DefaultAssetID,
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
	s.source.EXPECT().Read(mock.Anything).Return(telemetry.TelemetryReading{
		Setpoint:    10,
		ActivePower: 20,
	}, nil).Once()

	err := s.handler.Handle(s.ctx, collecttelemetry.CollectTelemetry{CollectedAt: s.collectedAt})

	s.Error(err)
	s.ErrorIs(err, collecttelemetry.ErrInvalidTelemetry)
	s.ErrorIs(err, telemetry.ErrInvalidMeasurement)
	s.repository.AssertNotCalled(s.T(), "Save", mock.Anything, mock.Anything)
}

func (s *CollectTelemetryHandlerSuite) TestSourceErrorIsReturned() {
	sourceErr := errors.New("source unavailable")

	s.source.EXPECT().Read(mock.Anything).Return(telemetry.TelemetryReading{}, sourceErr).Once()

	err := s.handler.Handle(s.ctx, collecttelemetry.CollectTelemetry{CollectedAt: s.collectedAt})

	s.Error(err)
	s.ErrorIs(err, collecttelemetry.ErrTelemetrySource)
	s.ErrorIs(err, sourceErr)
	s.repository.AssertNotCalled(s.T(), "Save", mock.Anything, mock.Anything)
}

func (s *CollectTelemetryHandlerSuite) TestRepositoryErrorIsReturned() {
	repositoryErr := errors.New("repository unavailable")
	expectedMeasurement := telemetry.Measurement{
		AssetID:     telemetry.DefaultAssetID,
		Setpoint:    100,
		ActivePower: 80,
		CollectedAt: s.collectedAt,
	}

	s.source.EXPECT().Read(mock.Anything).Return(telemetry.TelemetryReading{
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
		assetID    telemetry.AssetID
		source     telemetry.TelemetrySource
		repository telemetry.MeasurementRepository
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
			assetID:    telemetry.DefaultAssetID,
			source:     nil,
			repository: s.repository,
			wantErr:    collecttelemetry.ErrNilTelemetrySource,
		},
		{
			name:       "nil repository",
			assetID:    telemetry.DefaultAssetID,
			source:     s.source,
			repository: nil,
			wantErr:    collecttelemetry.ErrNilMeasurementRepository,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			handler, err := collecttelemetry.NewCollectTelemetryHandler(tc.assetID, tc.source, tc.repository)

			s.Error(err)
			s.ErrorIs(err, tc.wantErr)
			s.Equal(collecttelemetry.CollectTelemetryHandler{}, handler)
		})
	}
}
