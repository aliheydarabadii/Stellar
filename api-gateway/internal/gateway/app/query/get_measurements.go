package query

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"api_gateway/internal/gateway/requestctx"
)

var (
	ErrAssetIDRequired               = errors.New("asset id is required")
	ErrInvalidTimeRange              = errors.New("from must not be after to")
	ErrTimestampZero                 = errors.New("from and to must be set")
	ErrMeasurementsClientRequired    = errors.New("measurements client is required")
	ErrMeasurementsCacheRequired     = errors.New("measurements cache is required")
	ErrCacheKeyBuilderRequired       = errors.New("cache key builder is required")
	ErrCacheTTLInvalid               = errors.New("cache ttl must be positive")
	ErrMeasurementServiceUnavailable = errors.New("measurement service unavailable")
	ErrDownstreamInvalidRequest      = errors.New("measurement service rejected request")
)

type downstreamInvalidRequestError struct {
	message string
}

func NewDownstreamInvalidRequestError(message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		return ErrDownstreamInvalidRequest
	}

	return downstreamInvalidRequestError{message: message}
}

func (e downstreamInvalidRequestError) Error() string {
	return e.message
}

func (e downstreamInvalidRequestError) Is(target error) bool {
	return target == ErrDownstreamInvalidRequest
}

type GetMeasurements struct {
	AssetID string
	From    time.Time
	To      time.Time
}

type MeasurementsClient interface {
	GetMeasurements(ctx context.Context, assetID string, from, to time.Time) (MeasurementSeries, error)
}

type MeasurementsCache interface {
	Get(ctx context.Context, key string) (MeasurementSeries, bool, error)
	Set(ctx context.Context, key string, value MeasurementSeries, ttl time.Duration) error
}

type CacheKeyBuilder func(assetID string, from, to time.Time) string

type GetMeasurementsHandler struct {
	client        MeasurementsClient
	cache         MeasurementsCache
	cacheTTL      time.Duration
	buildCacheKey CacheKeyBuilder
	logger        *slog.Logger
}

func NewGetMeasurementsHandler(
	client MeasurementsClient,
	cache MeasurementsCache,
	cacheTTL time.Duration,
	buildCacheKey CacheKeyBuilder,
	logger *slog.Logger,
) (GetMeasurementsHandler, error) {
	switch {
	case client == nil:
		return GetMeasurementsHandler{}, ErrMeasurementsClientRequired
	case cache == nil:
		return GetMeasurementsHandler{}, ErrMeasurementsCacheRequired
	case buildCacheKey == nil:
		return GetMeasurementsHandler{}, ErrCacheKeyBuilderRequired
	case cacheTTL <= 0:
		return GetMeasurementsHandler{}, ErrCacheTTLInvalid
	}

	return GetMeasurementsHandler{
		client:        client,
		cache:         cache,
		cacheTTL:      cacheTTL,
		buildCacheKey: buildCacheKey,
		logger:        logger,
	}, nil
}

func (h GetMeasurementsHandler) Handle(ctx context.Context, qry GetMeasurements) (MeasurementSeries, error) {
	assetID := strings.TrimSpace(qry.AssetID)
	from := qry.From.UTC()
	to := qry.To.UTC()

	switch {
	case assetID == "":
		return MeasurementSeries{}, ErrAssetIDRequired
	case qry.From.IsZero() || qry.To.IsZero():
		return MeasurementSeries{}, ErrTimestampZero
	case qry.From.After(qry.To):
		return MeasurementSeries{}, ErrInvalidTimeRange
	}

	cacheKey := h.buildCacheKey(assetID, from, to)

	series, found, err := h.cache.Get(ctx, cacheKey)
	if err != nil {
		requestctx.SetCacheStatus(ctx, requestctx.CacheStatusBypass)
		h.logCacheWarning(ctx, "cache get failed", cacheKey, err)
	} else if found {
		requestctx.SetCacheStatus(ctx, requestctx.CacheStatusHit)
		return series, nil
	} else {
		requestctx.SetCacheStatus(ctx, requestctx.CacheStatusMiss)
	}

	series, err = h.client.GetMeasurements(ctx, assetID, from, to)
	if err != nil {
		return MeasurementSeries{}, err
	}

	if err := h.cache.Set(ctx, cacheKey, series, h.cacheTTL); err != nil {
		h.logCacheWarning(ctx, "cache set failed", cacheKey, err)
	}

	return series, nil
}

func (h GetMeasurementsHandler) logCacheWarning(ctx context.Context, message, key string, err error) {
	if h.logger == nil {
		return
	}

	h.logger.WarnContext(ctx, message, "key", key, "error", err)
}
