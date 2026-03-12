package getmeasurements

import (
	"context"
	"strings"
	"time"
)

type UseCase struct {
	client        MeasurementsClient
	cache         MeasurementsCache
	cacheTTL      time.Duration
	buildCacheKey CacheKeyBuilder
	observer      CacheFailureObserver
}

func NewUseCase(
	client MeasurementsClient,
	cache MeasurementsCache,
	cacheTTL time.Duration,
	buildCacheKey CacheKeyBuilder,
	observer CacheFailureObserver,
) (UseCase, error) {
	switch {
	case client == nil:
		return UseCase{}, ErrMeasurementsClientRequired
	case cache == nil:
		return UseCase{}, ErrMeasurementsCacheRequired
	case buildCacheKey == nil:
		return UseCase{}, ErrCacheKeyBuilderRequired
	case cacheTTL <= 0:
		return UseCase{}, ErrCacheTTLInvalid
	}

	return UseCase{
		client:        client,
		cache:         cache,
		cacheTTL:      cacheTTL,
		buildCacheKey: buildCacheKey,
		observer:      observer,
	}, nil
}

func (u UseCase) Handle(ctx context.Context, qry Query) (Result, error) {
	assetID := strings.TrimSpace(qry.AssetID)
	from := qry.From.UTC()
	to := qry.To.UTC()

	switch {
	case assetID == "":
		return Result{}, ErrAssetIDRequired
	case qry.From.IsZero() || qry.To.IsZero():
		return Result{}, ErrTimestampZero
	case qry.From.After(qry.To):
		return Result{}, ErrInvalidTimeRange
	}

	cacheKey := u.buildCacheKey(assetID, from, to)
	result := Result{CacheStatus: CacheStatusMiss}

	series, found, err := u.cache.Get(ctx, cacheKey)
	if err != nil {
		result.CacheStatus = CacheStatusBypass
		u.observeCacheFailure(ctx, "get", cacheKey, err)
	} else if found {
		return Result{
			Series:      series,
			CacheStatus: CacheStatusHit,
		}, nil
	}

	series, err = u.client.GetMeasurements(ctx, assetID, from, to)
	if err != nil {
		return Result{}, mapMeasurementsClientError(err)
	}

	if err := u.cache.Set(ctx, cacheKey, series, u.cacheTTL); err != nil {
		u.observeCacheFailure(ctx, "set", cacheKey, err)
	}

	result.Series = series
	return result, nil
}

func (u UseCase) observeCacheFailure(ctx context.Context, operation, key string, err error) {
	if u.observer == nil {
		return
	}

	u.observer.CacheOperationFailed(ctx, operation, key, err)
}
