package modbus

import (
	"context"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/suite"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	telemetry "stellar/internal/telemetry"
)

type InstrumentationTestSuite struct {
	suite.Suite
	tracer   trace.Tracer
	recorder *tracetest.SpanRecorder
}

func TestInstrumentationTestSuite(t *testing.T) {
	suite.Run(t, new(InstrumentationTestSuite))
}

func (s *InstrumentationTestSuite) SetupTest() {
	resetMetrics()
	s.tracer, s.recorder = newTestTracer()
}

func (s *InstrumentationTestSuite) TestInstrumentTelemetrySourceObservesReadDuration() {
	source := InstrumentTelemetrySource(stubTelemetrySource{
		reading: telemetry.TelemetryReading{
			Setpoint:    100,
			ActivePower: 50,
		},
	}, s.tracer)

	_, err := source.Read(context.Background())
	s.Require().NoError(err)

	s.Equal(uint64(1), histogramSampleCount(s.T(), sourceReadDuration))

	spans := s.recorder.Ended()
	s.Require().Len(spans, 1)
	s.Equal("telemetry.source.read", spans[0].Name())
}

type stubTelemetrySource struct {
	reading telemetry.TelemetryReading
	err     error
}

func (s stubTelemetrySource) Read(_ context.Context) (telemetry.TelemetryReading, error) {
	return s.reading, s.err
}

func newTestTracer() (trace.Tracer, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider()
	provider.RegisterSpanProcessor(recorder)

	return provider.Tracer("test"), recorder
}

func histogramSampleCount(t *testing.T, histogram interface{}) uint64 {
	t.Helper()

	metricWriter, ok := histogram.(interface{ Write(*dto.Metric) error })
	if !ok {
		t.Fatal("expected histogram to implement Write")
	}

	metric := &dto.Metric{}
	if err := metricWriter.Write(metric); err != nil {
		t.Fatalf("expected histogram metric to be writable, got %v", err)
	}

	return metric.GetHistogram().GetSampleCount()
}
