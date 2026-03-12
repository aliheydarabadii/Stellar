package getmeasurements

import (
	"context"
	"errors"
	"testing"
	"time"

	"api_gateway/internal/measurements/domain"
	"github.com/stretchr/testify/suite"
)

type UseCaseSuite struct {
	suite.Suite
}

func TestUseCaseSuite(t *testing.T) {
	suite.Run(t, new(UseCaseSuite))
}

func (s *UseCaseSuite) TestNewUseCaseRequiresReader() {
	_, err := NewUseCase(nil)

	s.ErrorIs(err, ErrMeasurementsReaderRequired)
}

func (s *UseCaseSuite) TestHandleRejectsMissingAssetID() {
	useCase := s.newUseCase(&stubReader{})

	_, err := useCase.Handle(context.Background(), Query{
		AssetID: " ",
		From:    s.baseTime(),
		To:      s.baseTime().Add(time.Minute),
	})

	s.ErrorIs(err, ErrAssetIDRequired)
}

func (s *UseCaseSuite) TestHandleRejectsInvalidRange() {
	useCase := s.newUseCase(&stubReader{})
	base := s.baseTime()

	_, err := useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    base.Add(time.Minute),
		To:      base,
	})

	s.ErrorIs(err, ErrInvalidTimeRange)
}

func (s *UseCaseSuite) TestHandleDelegatesToReaderWithUTCTimestamps() {
	location := time.FixedZone("UTC+02", 2*60*60)
	from := time.Date(2026, 3, 10, 14, 0, 0, 0, location)
	to := from.Add(time.Minute)
	reader := &stubReader{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}
	useCase := s.newUseCase(reader)

	series, err := useCase.Handle(context.Background(), Query{
		AssetID: " asset-1 ",
		From:    from,
		To:      to,
	})

	s.Require().NoError(err)
	s.Equal("asset-1", series.AssetID)
	s.Equal(1, reader.calls)
	s.Equal("asset-1", reader.assetID)
	s.Equal(time.UTC, reader.from.Location())
	s.Equal(time.UTC, reader.to.Location())
	s.True(reader.from.Equal(from.UTC()))
	s.True(reader.to.Equal(to.UTC()))
}

func (s *UseCaseSuite) TestHandleMapsMeasurementServiceUnavailable() {
	base := s.baseTime()
	useCase := s.newUseCase(&stubReader{
		err: stubUnavailableError{cause: errors.New("rpc unavailable")},
	})

	_, err := useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    base,
		To:      base.Add(time.Minute),
	})

	s.ErrorIs(err, ErrMeasurementServiceUnavailable)
}

func (s *UseCaseSuite) TestHandleMapsDownstreamInvalidRequest() {
	base := s.baseTime()
	useCase := s.newUseCase(&stubReader{
		err: stubInvalidRequestError{message: "window too large"},
	})

	_, err := useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    base,
		To:      base.Add(time.Minute),
	})

	s.ErrorIs(err, ErrDownstreamInvalidRequest)
	s.ErrorContains(err, "window too large")
}

func (s *UseCaseSuite) newUseCase(reader MeasurementsReader) UseCase {
	s.T().Helper()

	useCase, err := NewUseCase(reader)
	s.Require().NoError(err)

	return useCase
}

func (s *UseCaseSuite) baseTime() time.Time {
	return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
}

type stubReader struct {
	calls   int
	assetID string
	from    time.Time
	to      time.Time
	series  domain.MeasurementSeries
	err     error
}

func (r *stubReader) GetMeasurements(_ context.Context, assetID string, from, to time.Time) (domain.MeasurementSeries, error) {
	r.calls++
	r.assetID = assetID
	r.from = from
	r.to = to

	if r.err != nil {
		return domain.MeasurementSeries{}, r.err
	}

	if r.series.AssetID == "" {
		r.series.AssetID = assetID
	}

	return r.series, nil
}

type stubUnavailableError struct {
	cause error
}

func (e stubUnavailableError) Error() string {
	return "measurement service unavailable"
}

func (e stubUnavailableError) Unwrap() error {
	return e.cause
}

func (e stubUnavailableError) MeasurementServiceUnavailable() bool {
	return true
}

type stubInvalidRequestError struct {
	message string
}

func (e stubInvalidRequestError) Error() string {
	return e.message
}

func (e stubInvalidRequestError) DownstreamInvalidRequestMessage() string {
	return e.message
}
