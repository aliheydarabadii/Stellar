package influxdb

import (
	"context"
	"time"

	telemetry "stellar/internal/telemetry"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var persistenceDuration = newPersistenceDurationHistogram()

func MustRegisterMetrics(registerer prometheus.Registerer) {
	if registerer == nil {
		return
	}

	registerer.MustRegister(persistenceDuration)
}

func InstrumentMeasurementRepository(next telemetry.MeasurementRepository, tracer trace.Tracer) telemetry.MeasurementRepository {
	if next == nil {
		return next
	}

	if tracer == nil {
		tracer = otel.Tracer("stellar/internal/telemetry/adapters/outbound/influxdb")
	}

	return instrumentedMeasurementRepository{
		next:   next,
		tracer: tracer,
	}
}

type instrumentedMeasurementRepository struct {
	next   telemetry.MeasurementRepository
	tracer trace.Tracer
}

func (r instrumentedMeasurementRepository) Save(ctx context.Context, measurement telemetry.Measurement) error {
	ctx, span := r.tracer.Start(
		ctx,
		"telemetry.persistence.save",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("asset.id", measurement.AssetID.String())),
	)
	startedAt := time.Now()
	err := r.next.Save(ctx, measurement)
	persistenceDuration.Observe(time.Since(startedAt).Seconds())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return err
	}

	span.SetStatus(codes.Ok, "")
	span.End()

	return nil
}

func newPersistenceDurationHistogram() prometheus.Histogram {
	return prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "integration_service",
		Subsystem: "telemetry",
		Name:      "persistence_duration_seconds",
		Help:      "Telemetry persistence duration in seconds.",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})
}

func resetMetrics() {
	persistenceDuration = newPersistenceDurationHistogram()
}
