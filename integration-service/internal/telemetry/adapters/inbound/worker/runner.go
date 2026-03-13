// Package worker contains inbound worker execution for telemetry collection.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	healthplatform "stellar/internal/platform/health"
	telemetry "stellar/internal/telemetry"
	collecttelemetry "stellar/internal/telemetry/application"
)

type Worker interface {
	Start(ctx context.Context) error
}

type CollectTelemetryFunc func(ctx context.Context, cmd collecttelemetry.CollectTelemetry) error

var (
	collectionAttemptsTotal           = newCollectionAttemptsCounter()
	collectionSuccessTotal            = newCollectionSuccessCounter()
	collectionValidationFailuresTotal = newCollectionValidationFailuresCounter()
	collectionFailuresTotal           = newCollectionFailuresCounter()
	sourceFailuresTotal               = newSourceFailuresCounter()
	persistenceFailuresTotal          = newPersistenceFailuresCounter()
	collectionDurationSeconds         = newCollectionDurationHistogram()
	lastAttemptTimestampSeconds       = newLastAttemptGauge()
	lastSuccessTimestampSeconds       = newLastSuccessGauge()
)

func MustRegisterMetrics(registerer prometheus.Registerer) {
	if registerer == nil {
		return
	}

	registerer.MustRegister(
		collectionAttemptsTotal,
		collectionSuccessTotal,
		collectionValidationFailuresTotal,
		collectionFailuresTotal,
		sourceFailuresTotal,
		persistenceFailuresTotal,
		collectionDurationSeconds,
		lastAttemptTimestampSeconds,
		lastSuccessTimestampSeconds,
	)
}

type TickerWorker struct {
	logger    *slog.Logger
	interval  time.Duration
	handler   CollectTelemetryFunc
	readiness *healthplatform.Readiness
	tracer    trace.Tracer
}

func NewRunner(interval time.Duration, handler CollectTelemetryFunc, logger *slog.Logger, readiness *healthplatform.Readiness, tracer trace.Tracer) (*TickerWorker, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("worker interval must be positive")
	}

	if handler == nil {
		return nil, fmt.Errorf("worker handler must not be nil")
	}

	if logger == nil {
		logger = slog.Default()
	}

	if tracer == nil {
		tracer = otel.Tracer("stellar/internal/telemetry/adapters/inbound/worker")
	}

	return &TickerWorker{
		logger:    logger,
		interval:  interval,
		handler:   handler,
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
			recordAttempt(collectedAt)
			spanCtx, span := w.tracer.Start(
				ctx,
				"telemetry.collect",
				trace.WithAttributes(attribute.String("collected_at", collectedAt.Format(time.RFC3339Nano))),
			)
			err := w.handler(spanCtx, collecttelemetry.CollectTelemetry{
				CollectedAt: collectedAt,
			})
			collectionDurationSeconds.Observe(time.Since(startedAt).Seconds())
			if err == nil {
				recordSuccess(collectedAt)
				w.readiness.MarkSuccess(collectedAt)
				span.SetStatus(codes.Ok, "")
				span.End()
				continue
			}

			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()

			if errors.Is(err, collecttelemetry.ErrInvalidTelemetry) || errors.Is(err, telemetry.ErrInvalidMeasurement) {
				collectionValidationFailuresTotal.Inc()
				w.logger.Warn("telemetry validation failed; skipping persistence", "error", err, "collected_at", collectedAt)
				continue
			}

			collectionFailuresTotal.Inc()
			if errors.Is(err, collecttelemetry.ErrTelemetrySource) {
				sourceFailuresTotal.Inc()
			}
			if errors.Is(err, collecttelemetry.ErrMeasurementPersistence) {
				persistenceFailuresTotal.Inc()
			}

			w.logger.Error("telemetry collection failed", "error", err, "collected_at", collectedAt)
		}
	}
}

var _ Worker = (*TickerWorker)(nil)

func recordAttempt(collectedAt time.Time) {
	collectionAttemptsTotal.Inc()
	if !collectedAt.IsZero() {
		lastAttemptTimestampSeconds.Set(float64(collectedAt.Unix()))
	}
}

func recordSuccess(collectedAt time.Time) {
	collectionSuccessTotal.Inc()
	if !collectedAt.IsZero() {
		lastSuccessTimestampSeconds.Set(float64(collectedAt.Unix()))
	}
}

func newCollectionAttemptsCounter() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "integration_service",
		Subsystem: "telemetry",
		Name:      "collection_attempts_total",
		Help:      "Total telemetry collection attempts.",
	})
}

func newCollectionSuccessCounter() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "integration_service",
		Subsystem: "telemetry",
		Name:      "collection_success_total",
		Help:      "Total successful telemetry collections.",
	})
}

func newCollectionValidationFailuresCounter() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "integration_service",
		Subsystem: "telemetry",
		Name:      "collection_validation_failures_total",
		Help:      "Total telemetry collections rejected by domain validation.",
	})
}

func newCollectionFailuresCounter() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "integration_service",
		Subsystem: "telemetry",
		Name:      "collection_failures_total",
		Help:      "Total telemetry collection failures.",
	})
}

func newSourceFailuresCounter() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "integration_service",
		Subsystem: "telemetry",
		Name:      "source_failures_total",
		Help:      "Total telemetry source failures.",
	})
}

func newPersistenceFailuresCounter() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "integration_service",
		Subsystem: "telemetry",
		Name:      "persistence_failures_total",
		Help:      "Total telemetry persistence failures.",
	})
}

func newCollectionDurationHistogram() prometheus.Histogram {
	return prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "integration_service",
		Subsystem: "telemetry",
		Name:      "collection_duration_seconds",
		Help:      "End-to-end telemetry collection duration in seconds.",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})
}

func newLastAttemptGauge() prometheus.Gauge {
	return prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "integration_service",
		Subsystem: "telemetry",
		Name:      "last_attempt_timestamp_seconds",
		Help:      "Unix timestamp of the last telemetry collection attempt.",
	})
}

func newLastSuccessGauge() prometheus.Gauge {
	return prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "integration_service",
		Subsystem: "telemetry",
		Name:      "last_success_timestamp_seconds",
		Help:      "Unix timestamp of the last successful telemetry collection.",
	})
}

func resetMetrics() {
	collectionAttemptsTotal = newCollectionAttemptsCounter()
	collectionSuccessTotal = newCollectionSuccessCounter()
	collectionValidationFailuresTotal = newCollectionValidationFailuresCounter()
	collectionFailuresTotal = newCollectionFailuresCounter()
	sourceFailuresTotal = newSourceFailuresCounter()
	persistenceFailuresTotal = newPersistenceFailuresCounter()
	collectionDurationSeconds = newCollectionDurationHistogram()
	lastAttemptTimestampSeconds = newLastAttemptGauge()
	lastSuccessTimestampSeconds = newLastSuccessGauge()
}
