package ports

import (
	"context"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"stellar/internal/telemetry/app/command"
	"stellar/internal/telemetry/domain"
)

func TestInstrumentTelemetrySourceObservesReadDuration(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics()
	tracer, recorder := newTestTracer()
	source := InstrumentTelemetrySource(stubTelemetrySource{
		reading: command.TelemetryReading{
			Setpoint:    100,
			ActivePower: 50,
		},
	}, metrics, tracer)

	_, err := source.Read(context.Background())
	if err != nil {
		t.Fatalf("expected read to succeed, got %v", err)
	}

	if got := histogramSampleCount(t, metrics.sourceReadDuration); got != 1 {
		t.Fatalf("expected 1 source read duration sample, got %d", got)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "telemetry.source.read" {
		t.Fatalf("expected span name %q, got %q", "telemetry.source.read", spans[0].Name())
	}
}

func TestInstrumentMeasurementRepositoryObservesPersistenceDuration(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics()
	tracer, recorder := newTestTracer()
	repository := InstrumentMeasurementRepository(stubMeasurementRepository{}, metrics, tracer)

	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 50, time.Now().UTC())
	if err != nil {
		t.Fatalf("expected measurement to be valid, got %v", err)
	}

	if err := repository.Save(context.Background(), measurement); err != nil {
		t.Fatalf("expected save to succeed, got %v", err)
	}

	if got := histogramSampleCount(t, metrics.persistenceDuration); got != 1 {
		t.Fatalf("expected 1 persistence duration sample, got %d", got)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "telemetry.persistence.save" {
		t.Fatalf("expected span name %q, got %q", "telemetry.persistence.save", spans[0].Name())
	}
}

type stubTelemetrySource struct {
	reading command.TelemetryReading
	err     error
}

func (s stubTelemetrySource) Read(_ context.Context) (command.TelemetryReading, error) {
	return s.reading, s.err
}

type stubMeasurementRepository struct {
	err error
}

func (r stubMeasurementRepository) Save(_ context.Context, _ domain.Measurement) error {
	return r.err
}

func newTestTracer() (trace.Tracer, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider()
	provider.RegisterSpanProcessor(recorder)

	return provider.Tracer("test"), recorder
}
