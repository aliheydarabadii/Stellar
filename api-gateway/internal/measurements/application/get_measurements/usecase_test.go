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

func (s *UseCaseSuite) TestHandleReturnsCacheHit() {
	base := s.baseTime()
	cache := &stubCache{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
		found:  true,
	}
	client := &stubClient{}

	useCase := s.newUseCase(client, cache, nil)
	result, err := useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    base,
		To:      base.Add(time.Minute),
	})

	s.Require().NoError(err)
	s.Equal(CacheStatusHit, result.CacheStatus)
	s.Equal("asset-1", result.Series.AssetID)
	s.Equal(0, client.calls)
}

func (s *UseCaseSuite) TestHandleReturnsCacheMissAndStoresFetchedSeries() {
	base := s.baseTime()
	cache := &stubCache{}
	client := &stubClient{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}

	useCase := s.newUseCase(client, cache, nil)
	result, err := useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    base,
		To:      base.Add(time.Minute),
	})

	s.Require().NoError(err)
	s.Equal(CacheStatusMiss, result.CacheStatus)
	s.Equal(1, client.calls)
	s.Equal(1, cache.setCalls)
	s.Equal("asset-1", cache.series.AssetID)
}

func (s *UseCaseSuite) TestHandleBypassesCacheWhenGetFails() {
	base := s.baseTime()
	cache := &stubCache{getErr: errors.New("redis unavailable")}
	client := &stubClient{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}
	observer := &stubObserver{}

	useCase := s.newUseCase(client, cache, observer)
	result, err := useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    base,
		To:      base.Add(time.Minute),
	})

	s.Require().NoError(err)
	s.Equal(CacheStatusBypass, result.CacheStatus)
	s.Len(observer.operations, 1)
	s.Equal("get", observer.operations[0])
}

func (s *UseCaseSuite) TestHandleMapsMeasurementServiceUnavailable() {
	base := s.baseTime()
	useCase := s.newUseCase(&stubClient{
		err: stubUnavailableError{cause: errors.New("rpc unavailable")},
	}, &stubCache{}, nil)

	_, err := useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    base,
		To:      base.Add(time.Minute),
	})

	s.ErrorIs(err, ErrMeasurementServiceUnavailable)
}

func (s *UseCaseSuite) TestHandleMapsDownstreamInvalidRequest() {
	base := s.baseTime()
	useCase := s.newUseCase(&stubClient{
		err: stubInvalidRequestError{message: "window too large"},
	}, &stubCache{}, nil)

	_, err := useCase.Handle(context.Background(), Query{
		AssetID: "asset-1",
		From:    base,
		To:      base.Add(time.Minute),
	})

	s.ErrorIs(err, ErrDownstreamInvalidRequest)
	s.ErrorContains(err, "window too large")
}

func (s *UseCaseSuite) newUseCase(client MeasurementsClient, cache MeasurementsCache, observer CacheFailureObserver) UseCase {
	s.T().Helper()

	useCase, err := NewUseCase(
		client,
		cache,
		5*time.Minute,
		func(assetID string, from, to time.Time) string {
			return assetID + "|" + from.Format(time.RFC3339Nano) + "|" + to.Format(time.RFC3339Nano)
		},
		observer,
	)
	s.Require().NoError(err)

	return useCase
}

func (s *UseCaseSuite) baseTime() time.Time {
	return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
}

type stubClient struct {
	calls  int
	series domain.MeasurementSeries
	err    error
}

func (c *stubClient) GetMeasurements(_ context.Context, assetID string, _, _ time.Time) (domain.MeasurementSeries, error) {
	c.calls++
	if c.err != nil {
		return domain.MeasurementSeries{}, c.err
	}

	if c.series.AssetID == "" {
		c.series.AssetID = assetID
	}

	return c.series, nil
}

type stubCache struct {
	series   domain.MeasurementSeries
	found    bool
	getErr   error
	setErr   error
	setCalls int
}

func (c *stubCache) Get(_ context.Context, _ string) (domain.MeasurementSeries, bool, error) {
	if c.getErr != nil {
		return domain.MeasurementSeries{}, false, c.getErr
	}

	return c.series, c.found, nil
}

func (c *stubCache) Set(_ context.Context, _ string, value domain.MeasurementSeries, _ time.Duration) error {
	c.setCalls++
	if c.setErr != nil {
		return c.setErr
	}

	c.series = value
	c.found = true
	return nil
}

type stubObserver struct {
	operations []string
}

func (o *stubObserver) CacheOperationFailed(_ context.Context, operation, _ string, _ error) {
	o.operations = append(o.operations, operation)
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
