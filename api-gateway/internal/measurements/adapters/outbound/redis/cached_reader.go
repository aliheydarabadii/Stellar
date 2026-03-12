package redis

import (
	"context"
	"time"

	"api_gateway/internal/measurements"
	getmeasurements "api_gateway/internal/measurements/application"
	"api_gateway/internal/platform/requestctx"
)

type CachedReader struct {
	inner         measurements.MeasurementsReader
	cache         getmeasurements.MeasurementsCache
	cacheTTL      time.Duration
	buildCacheKey getmeasurements.CacheKeyBuilder
	observer      getmeasurements.CacheFailureObserver
}

func NewCachedReader(
	inner measurements.MeasurementsReader,
	cache getmeasurements.MeasurementsCache,
	cacheTTL time.Duration,
	buildCacheKey getmeasurements.CacheKeyBuilder,
	observer getmeasurements.CacheFailureObserver,
) (*CachedReader, error) {
	switch {
	case inner == nil:
		return nil, getmeasurements.ErrMeasurementsReaderRequired
	case cache == nil:
		return nil, getmeasurements.ErrMeasurementsCacheRequired
	case buildCacheKey == nil:
		return nil, getmeasurements.ErrCacheKeyBuilderRequired
	case cacheTTL <= 0:
		return nil, getmeasurements.ErrCacheTTLInvalid
	}

	return &CachedReader{
		inner:         inner,
		cache:         cache,
		cacheTTL:      cacheTTL,
		buildCacheKey: buildCacheKey,
		observer:      observer,
	}, nil
}

func (r *CachedReader) GetMeasurements(ctx context.Context, assetID string, from, to time.Time) (measurements.MeasurementSeries, error) {
	cacheKey := r.buildCacheKey(assetID, from, to)

	series, found, err := r.cache.Get(ctx, cacheKey)
	if err != nil {
		requestctx.SetCacheStatus(ctx, requestctx.CacheStatusBypass)
		r.observeCacheFailure(ctx, "get", cacheKey, err)
		return r.inner.GetMeasurements(ctx, assetID, from, to)
	}
	if found {
		requestctx.SetCacheStatus(ctx, requestctx.CacheStatusHit)
		return series, nil
	}

	requestctx.SetCacheStatus(ctx, requestctx.CacheStatusMiss)

	series, err = r.inner.GetMeasurements(ctx, assetID, from, to)
	if err != nil {
		return measurements.MeasurementSeries{}, err
	}

	if err := r.cache.Set(ctx, cacheKey, series, r.cacheTTL); err != nil {
		r.observeCacheFailure(ctx, "set", cacheKey, err)
	}

	return series, nil
}

func (r *CachedReader) observeCacheFailure(ctx context.Context, operation, key string, err error) {
	if r.observer == nil {
		return
	}

	r.observer.CacheOperationFailed(ctx, operation, key, err)
}
