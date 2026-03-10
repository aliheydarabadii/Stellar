package ports

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"stellar/internal/telemetry/app/command"
	"stellar/internal/telemetry/domain"
)

type Worker interface {
	Start(ctx context.Context) error
}

type collectTelemetryHandler interface {
	Handle(ctx context.Context, cmd command.CollectTelemetry) error
}

type TickerWorker struct {
	logger   *slog.Logger
	interval time.Duration
	handler  collectTelemetryHandler
}

func NewTickerWorker(interval time.Duration, handler collectTelemetryHandler, logger *slog.Logger) (*TickerWorker, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("worker interval must be positive")
	}

	if handler == nil {
		return nil, fmt.Errorf("worker handler must not be nil")
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &TickerWorker{
		logger:   logger,
		interval: interval,
		handler:  handler,
	}, nil
}

func (w *TickerWorker) Start(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.logger.Info("telemetry worker started", "interval", w.interval.String())

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("telemetry worker stopped")
			return nil
		case <-ticker.C:
			collectedAt := time.Now().UTC()
			err := w.handler.Handle(ctx, command.CollectTelemetry{
				CollectedAt: collectedAt,
			})
			if err == nil {
				continue
			}

			if errors.Is(err, command.ErrInvalidTelemetry) || errors.Is(err, domain.ErrInvalidMeasurement) {
				w.logger.Warn("telemetry validation failed; skipping persistence", "error", err, "collected_at", collectedAt)
				continue
			}

			w.logger.Error("telemetry collection failed", "error", err, "collected_at", collectedAt)
		}
	}
}

var _ Worker = (*TickerWorker)(nil)
