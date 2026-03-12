package getmeasurements

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"api_gateway/internal/measurements/domain"
	"api_gateway/internal/platform/requestctx"
)

type UseCase struct {
	client        MeasurementsClient
	cache         MeasurementsCache
	cacheTTL      time.Duration
	buildCacheKey CacheKeyBuilder
	logger        *slog.Logger
}

func NewUseCase(
	client MeasurementsClient,
	cache MeasurementsCache,
	cacheTTL time.Duration,
	buildCacheKey CacheKeyBuilder,
	logger *slog.Logger,
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
		logger:        logger,
	}, nil
}

func (u UseCase) Handle(ctx context.Context, qry Query) (domain.MeasurementSeries, error) {
	assetID := strings.TrimSpace(qry.AssetID)
	from := qry.From.UTC()
	to := qry.To.UTC()

	switch {
	case assetID == "":
		return domain.MeasurementSeries{}, ErrAssetIDRequired
	case qry.From.IsZero() || qry.To.IsZero():
		return domain.MeasurementSeries{}, ErrTimestampZero
	case qry.From.After(qry.To):
		return domain.MeasurementSeries{}, ErrInvalidTimeRange
	}

	cacheKey := u.buildCacheKey(assetID, from, to)

	series, found, err := u.cache.Get(ctx, cacheKey)
	if err != nil {
		requestctx.SetCacheStatus(ctx, requestctx.CacheStatusBypass)
		u.logCacheWarning(ctx, "cache get failed", cacheKey, err)
	} else if found {
		requestctx.SetCacheStatus(ctx, requestctx.CacheStatusHit)
		return series, nil
	} else {
		requestctx.SetCacheStatus(ctx, requestctx.CacheStatusMiss)
	}

	series, err = u.client.GetMeasurements(ctx, assetID, from, to)
	if err != nil {
		return domain.MeasurementSeries{}, err
	}

	if err := u.cache.Set(ctx, cacheKey, series, u.cacheTTL); err != nil {
		u.logCacheWarning(ctx, "cache set failed", cacheKey, err)
	}

	return series, nil
}

func (u UseCase) logCacheWarning(ctx context.Context, message, key string, err error) {
	if u.logger == nil {
		return
	}

	u.logger.WarnContext(ctx, message, "key", key, "error", err)
}
