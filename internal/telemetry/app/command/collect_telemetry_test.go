package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	command "stellar/internal/telemetry/app/command"
	commandmocks "stellar/internal/telemetry/app/command/mocks"
	"stellar/internal/telemetry/domain"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type CollectTelemetryHandlerSuite struct {
	suite.Suite
	ctx         context.Context
	collectedAt time.Time
	source      *commandmocks.TelemetrySource
	repository  *commandmocks.MeasurementRepository
	handler     command.CollectTelemetryHandler
}

func TestCollectTelemetryHandlerSuite(t *testing.T) {
	t.Parallel()

	suite.Run(t, new(CollectTelemetryHandlerSuite))
}

func (s *CollectTelemetryHandlerSuite) SetupTest() {
	s.ctx = context.Background()
	s.collectedAt = time.Date(2026, time.March, 9, 12, 0, 0, 0, time.UTC)
	s.source = commandmocks.NewTelemetrySource(s.T())
	s.repository = commandmocks.NewMeasurementRepository(s.T())
	s.handler = command.NewCollectTelemetryHandler(domain.DefaultAssetID, s.source, s.repository)
}

func (s *CollectTelemetryHandlerSuite) TestValidReadingGetsSaved() {
	reading := command.TelemetryReading{
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

	err := s.handler.Handle(s.ctx, command.CollectTelemetry{CollectedAt: s.collectedAt})

	s.NoError(err)
}

func (s *CollectTelemetryHandlerSuite) TestInvalidReadingDoesNotGetSaved() {
	s.source.EXPECT().Read(mock.Anything).Return(command.TelemetryReading{
		Setpoint:    10,
		ActivePower: 20,
	}, nil).Once()

	err := s.handler.Handle(s.ctx, command.CollectTelemetry{CollectedAt: s.collectedAt})

	s.Error(err)
	s.ErrorIs(err, command.ErrInvalidTelemetry)
	s.ErrorIs(err, domain.ErrInvalidMeasurement)
	s.repository.AssertNotCalled(s.T(), "Save", mock.Anything, mock.Anything)
}

func (s *CollectTelemetryHandlerSuite) TestSourceErrorIsReturned() {
	sourceErr := errors.New("source unavailable")

	s.source.EXPECT().Read(mock.Anything).Return(command.TelemetryReading{}, sourceErr).Once()

	err := s.handler.Handle(s.ctx, command.CollectTelemetry{CollectedAt: s.collectedAt})

	s.Error(err)
	s.ErrorIs(err, command.ErrTelemetrySource)
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

	s.source.EXPECT().Read(mock.Anything).Return(command.TelemetryReading{
		Setpoint:    100,
		ActivePower: 80,
	}, nil).Once()
	s.repository.EXPECT().Save(mock.Anything, expectedMeasurement).Return(repositoryErr).Once()

	err := s.handler.Handle(s.ctx, command.CollectTelemetry{CollectedAt: s.collectedAt})

	s.Error(err)
	s.ErrorIs(err, command.ErrMeasurementPersistence)
	s.ErrorIs(err, repositoryErr)
}
