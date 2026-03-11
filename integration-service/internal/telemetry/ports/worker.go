// Package ports contains runtime ports for driving the application.
package ports

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
	logger    *slog.Logger
	interval  time.Duration
	handler   collectTelemetryHandler
	metrics   *Metrics
	readiness *Readiness
	tracer    trace.Tracer
}

func NewTickerWorker(interval time.Duration, handler collectTelemetryHandler, logger *slog.Logger, metrics *Metrics, readiness *Readiness, tracer trace.Tracer) (*TickerWorker, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("worker interval must be positive")
	}

	if handler == nil {
		return nil, fmt.Errorf("worker handler must not be nil")
	}

	if logger == nil {
		logger = slog.Default()
	}

	if metrics == nil {
		metrics = NewMetrics()
	}

	if tracer == nil {
		tracer = otel.Tracer("stellar/internal/telemetry/ports")
	}

	return &TickerWorker{
		logger:    logger,
		interval:  interval,
		handler:   handler,
		metrics:   metrics,
		readiness: readiness,
		tracer:    tracer,
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
			startedAt := time.Now()
			w.metrics.RecordAttempt(collectedAt)
			spanCtx, span := w.tracer.Start(
				ctx,
				"telemetry.collect",
				trace.WithAttributes(attribute.String("collected_at", collectedAt.Format(time.RFC3339Nano))),
			)
			err := w.handler.Handle(spanCtx, command.CollectTelemetry{
				CollectedAt: collectedAt,
			})
			w.metrics.ObserveCollectionDuration(time.Since(startedAt))
			if err == nil {
				w.metrics.RecordSuccess(collectedAt)
				w.readiness.MarkSuccess(collectedAt)
				span.SetStatus(codes.Ok, "")
				span.End()
				continue
			}

			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()

			if errors.Is(err, command.ErrInvalidTelemetry) || errors.Is(err, domain.ErrInvalidMeasurement) {
				w.metrics.RecordValidationFailure()
				w.logger.Warn("telemetry validation failed; skipping persistence", "error", err, "collected_at", collectedAt)
				continue
			}

			w.metrics.RecordFailure()
			if errors.Is(err, command.ErrTelemetrySource) {
				w.metrics.RecordSourceFailure()
			}
			if errors.Is(err, command.ErrMeasurementPersistence) {
				w.metrics.RecordPersistenceFailure()
			}

			w.logger.Error("telemetry collection failed", "error", err, "collected_at", collectedAt)
		}
	}
}

var _ Worker = (*TickerWorker)(nil)
