package modbus

import (
	"context"
	"time"

	telemetry "stellar/internal/telemetry"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var sourceReadDuration = newSourceReadDurationHistogram()

func MustRegisterMetrics(registerer prometheus.Registerer) {
	if registerer == nil {
		return
	}

	registerer.MustRegister(sourceReadDuration)
}

func InstrumentTelemetrySource(next telemetry.TelemetrySource, tracer trace.Tracer) telemetry.TelemetrySource {
	if next == nil {
		return next
	}

	if tracer == nil {
		tracer = otel.Tracer("stellar/internal/telemetry/adapters/outbound/modbus")
	}

	return instrumentedTelemetrySource{
		next:   next,
		tracer: tracer,
	}
}

type instrumentedTelemetrySource struct {
	next   telemetry.TelemetrySource
	tracer trace.Tracer
}

func (s instrumentedTelemetrySource) Read(ctx context.Context) (telemetry.TelemetryReading, error) {
	ctx, span := s.tracer.Start(ctx, "telemetry.source.read", trace.WithSpanKind(trace.SpanKindClient))
	startedAt := time.Now()
	reading, err := s.next.Read(ctx)
	sourceReadDuration.Observe(time.Since(startedAt).Seconds())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return reading, err
	}

	span.SetStatus(codes.Ok, "")
	span.End()

	return reading, nil
}

func newSourceReadDurationHistogram() prometheus.Histogram {
	return prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "integration_service",
		Subsystem: "telemetry",
		Name:      "source_read_duration_seconds",
		Help:      "Telemetry source read duration in seconds.",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})
}

func resetMetrics() {
	sourceReadDuration = newSourceReadDurationHistogram()
}
