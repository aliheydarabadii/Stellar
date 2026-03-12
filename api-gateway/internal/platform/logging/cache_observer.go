package logging

import (
	"context"
	"log/slog"
)

type CacheObserver struct {
	logger *slog.Logger
}

func NewCacheObserver(logger *slog.Logger) *CacheObserver {
	if logger == nil {
		return nil
	}

	return &CacheObserver{logger: logger}
}

func (o *CacheObserver) CacheOperationFailed(ctx context.Context, operation, key string, err error) {
	if o == nil || o.logger == nil {
		return
	}

	o.logger.WarnContext(ctx, "cache operation failed", "operation", operation, "key", key, "error", err)
}
