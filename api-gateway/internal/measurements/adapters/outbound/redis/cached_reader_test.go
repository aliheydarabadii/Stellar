package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	getmeasurements "api_gateway/internal/measurements/application/get_measurements"
	"api_gateway/internal/measurements/domain"
	"api_gateway/internal/platform/requestctx"
	"github.com/stretchr/testify/suite"
)

type CachedReaderSuite struct {
	suite.Suite
}

func TestCachedReaderSuite(t *testing.T) {
	suite.Run(t, new(CachedReaderSuite))
}

func (s *CachedReaderSuite) TestNewCachedReaderRejectsInvalidArgs() {
	baseReader := &stubMeasurementsReader{}
	baseCache := &stubMeasurementsCache{}

	_, err := NewCachedReader(nil, baseCache, time.Minute, MeasurementsKey, nil)
	s.ErrorIs(err, getmeasurements.ErrMeasurementsReaderRequired)

	_, err = NewCachedReader(baseReader, nil, time.Minute, MeasurementsKey, nil)
	s.ErrorIs(err, getmeasurements.ErrMeasurementsCacheRequired)

	_, err = NewCachedReader(baseReader, baseCache, 0, MeasurementsKey, nil)
	s.ErrorIs(err, getmeasurements.ErrCacheTTLInvalid)

	_, err = NewCachedReader(baseReader, baseCache, time.Minute, nil, nil)
	s.ErrorIs(err, getmeasurements.ErrCacheKeyBuilderRequired)
}

func (s *CachedReaderSuite) TestGetMeasurementsReturnsCacheHitAndSkipsInnerReader() {
	ctx := requestctx.WithValues(context.Background(), "req-1", "corr-1")
	cachedSeries := domain.MeasurementSeries{AssetID: "asset-1"}
	reader := &stubMeasurementsReader{}
	cache := &stubMeasurementsCache{
		series: cachedSeries,
		found:  true,
	}

	decorator, err := NewCachedReader(reader, cache, 5*time.Minute, MeasurementsKey, nil)
	s.Require().NoError(err)

	series, err := decorator.GetMeasurements(ctx, "asset-1", s.baseTime(), s.baseTime().Add(time.Minute))

	s.Require().NoError(err)
	s.Equal(cachedSeries, series)
	s.Equal(0, reader.calls)
	s.Equal(requestctx.CacheStatusHit, requestctx.CacheStatusFromContext(ctx))
}

func (s *CachedReaderSuite) TestGetMeasurementsReturnsCacheMissStoresValueAndWritesStatus() {
	ctx := requestctx.WithValues(context.Background(), "req-1", "corr-1")
	reader := &stubMeasurementsReader{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}
	cache := &stubMeasurementsCache{}

	decorator, err := NewCachedReader(reader, cache, 5*time.Minute, MeasurementsKey, nil)
	s.Require().NoError(err)

	series, err := decorator.GetMeasurements(ctx, "asset-1", s.baseTime(), s.baseTime().Add(time.Minute))

	s.Require().NoError(err)
	s.Equal("asset-1", series.AssetID)
	s.Equal(1, reader.calls)
	s.Equal(1, cache.setCalls)
	s.Equal(requestctx.CacheStatusMiss, requestctx.CacheStatusFromContext(ctx))
}

func (s *CachedReaderSuite) TestGetMeasurementsBypassesCacheGetFailure() {
	ctx := requestctx.WithValues(context.Background(), "req-1", "corr-1")
	reader := &stubMeasurementsReader{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}
	cache := &stubMeasurementsCache{
		getErr: errors.New("redis unavailable"),
	}
	observer := &stubCacheFailureObserver{}

	decorator, err := NewCachedReader(reader, cache, 5*time.Minute, MeasurementsKey, observer)
	s.Require().NoError(err)

	series, err := decorator.GetMeasurements(ctx, "asset-1", s.baseTime(), s.baseTime().Add(time.Minute))

	s.Require().NoError(err)
	s.Equal("asset-1", series.AssetID)
	s.Equal(1, reader.calls)
	s.Equal(0, cache.setCalls)
	s.Equal([]string{"get"}, observer.operations)
	s.Equal(requestctx.CacheStatusBypass, requestctx.CacheStatusFromContext(ctx))
}

func (s *CachedReaderSuite) TestGetMeasurementsDoesNotFailWhenCacheSetFails() {
	ctx := requestctx.WithValues(context.Background(), "req-1", "corr-1")
	reader := &stubMeasurementsReader{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}
	cache := &stubMeasurementsCache{
		setErr: errors.New("redis write failed"),
	}
	observer := &stubCacheFailureObserver{}

	decorator, err := NewCachedReader(reader, cache, 5*time.Minute, MeasurementsKey, observer)
	s.Require().NoError(err)

	series, err := decorator.GetMeasurements(ctx, "asset-1", s.baseTime(), s.baseTime().Add(time.Minute))

	s.Require().NoError(err)
	s.Equal("asset-1", series.AssetID)
	s.Equal(1, cache.setCalls)
	s.Equal([]string{"set"}, observer.operations)
	s.Equal(requestctx.CacheStatusMiss, requestctx.CacheStatusFromContext(ctx))
}

func (s *CachedReaderSuite) baseTime() time.Time {
	return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
}

type stubMeasurementsReader struct {
	calls  int
	series domain.MeasurementSeries
	err    error
}

func (r *stubMeasurementsReader) GetMeasurements(_ context.Context, assetID string, _, _ time.Time) (domain.MeasurementSeries, error) {
	r.calls++
	if r.err != nil {
		return domain.MeasurementSeries{}, r.err
	}

	if r.series.AssetID == "" {
		r.series.AssetID = assetID
	}

	return r.series, nil
}

type stubMeasurementsCache struct {
	series   domain.MeasurementSeries
	found    bool
	getErr   error
	setErr   error
	setCalls int
}

func (c *stubMeasurementsCache) Get(_ context.Context, _ string) (domain.MeasurementSeries, bool, error) {
	if c.getErr != nil {
		return domain.MeasurementSeries{}, false, c.getErr
	}

	return c.series, c.found, nil
}

func (c *stubMeasurementsCache) Set(_ context.Context, _ string, value domain.MeasurementSeries, _ time.Duration) error {
	c.setCalls++
	if c.setErr != nil {
		return c.setErr
	}

	c.series = value
	c.found = true
	return nil
}

type stubCacheFailureObserver struct {
	operations []string
}

func (o *stubCacheFailureObserver) CacheOperationFailed(_ context.Context, operation, _ string, _ error) {
	o.operations = append(o.operations, operation)
}
