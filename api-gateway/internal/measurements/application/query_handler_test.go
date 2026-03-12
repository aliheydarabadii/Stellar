package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"api_gateway/internal/measurements"
	measurementsmocks "api_gateway/internal/measurements/mocks"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type GetMeasurementsHandlerSuite struct {
	suite.Suite
}

func TestGetMeasurementsHandlerSuite(t *testing.T) {
	suite.Run(t, new(GetMeasurementsHandlerSuite))
}

func (s *GetMeasurementsHandlerSuite) TestNewUseCaseRequiresReader() {
	_, err := NewMeasurementsHandler(nil)

	s.ErrorIs(err, ErrMeasurementsReaderRequired)
}

func (s *GetMeasurementsHandlerSuite) TestHandleRejectsMissingAssetID() {
	useCase := s.newUseCase(measurementsmocks.NewMeasurementsReader(s.T()))

	_, err := useCase.Handle(context.Background(), Query{
		AssetID: " ",
		From:    s.baseTime(),
		To:      s.baseTime().Add(time.Minute),
	})

	s.ErrorIs(err, ErrAssetIDRequired)
}

func (s *GetMeasurementsHandlerSuite) TestHandleRejectsInvalidRange() {
	useCase := s.newUseCase(measurementsmocks.NewMeasurementsReader(s.T()))
	base := s.baseTime()

	_, err := useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    base.Add(time.Minute),
		To:      base,
	})

	s.ErrorIs(err, ErrInvalidTimeRange)
}

func (s *GetMeasurementsHandlerSuite) TestHandleDelegatesToReaderWithUTCTimestamps() {
	location := time.FixedZone("UTC+02", 2*60*60)
	from := time.Date(2026, 3, 10, 14, 0, 0, 0, location)
	to := from.Add(time.Minute)
	reader := measurementsmocks.NewMeasurementsReader(s.T())
	reader.EXPECT().
		GetMeasurements(mock.Anything, "asset-1", from.UTC(), to.UTC()).
		Return(measurements.MeasurementSeries{AssetID: "asset-1"}, nil).
		Once()
	useCase := s.newUseCase(reader)

	series, err := useCase.Handle(context.Background(), Query{
		AssetID: " asset-1 ",
		From:    from,
		To:      to,
	})

	s.Require().NoError(err)
	s.Equal("asset-1", series.AssetID)
}

func (s *GetMeasurementsHandlerSuite) TestHandleMapsMeasurementServiceUnavailable() {
	base := s.baseTime()
	reader := measurementsmocks.NewMeasurementsReader(s.T())
	reader.EXPECT().
		GetMeasurements(mock.Anything, "asset-1", base, base.Add(time.Minute)).
		Return(measurements.MeasurementSeries{}, fmt.Errorf("%w: %w", measurements.ErrMeasurementsReaderUnavailable, errors.New("rpc unavailable"))).
		Once()
	useCase := s.newUseCase(reader)

	_, err := useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    base,
		To:      base.Add(time.Minute),
	})

	s.ErrorIs(err, ErrMeasurementServiceUnavailable)
}

func (s *GetMeasurementsHandlerSuite) TestHandleMapsDownstreamInvalidRequest() {
	base := s.baseTime()
	reader := measurementsmocks.NewMeasurementsReader(s.T())
	reader.EXPECT().
		GetMeasurements(mock.Anything, "asset-1", base, base.Add(time.Minute)).
		Return(measurements.MeasurementSeries{}, fmt.Errorf("%w: %s", measurements.ErrMeasurementsReaderInvalidRequest, "window too large")).
		Once()
	useCase := s.newUseCase(reader)

	_, err := useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    base,
		To:      base.Add(time.Minute),
	})

	s.ErrorIs(err, ErrDownstreamInvalidRequest)
	s.ErrorContains(err, "window too large")
}

func (s *GetMeasurementsHandlerSuite) newUseCase(reader measurements.MeasurementsReader) GetMeasurementsHandler {
	s.T().Helper()

	useCase, err := NewMeasurementsHandler(reader)
	s.Require().NoError(err)

	return useCase
}

func (s *GetMeasurementsHandlerSuite) baseTime() time.Time {
	return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
}
