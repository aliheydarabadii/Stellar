package influxdb

import (
	"context"
	"testing"
	"time"

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

func (s *InstrumentationTestSuite) TestInstrumentMeasurementRepositoryObservesPersistenceDuration() {
	repository := InstrumentMeasurementRepository(stubMeasurementRepository{}, s.tracer)

	measurement, err := telemetry.NewMeasurement(telemetry.DefaultAssetID, 100, 50, time.Now().UTC())
	s.Require().NoError(err)

	s.Require().NoError(repository.Save(context.Background(), measurement))
	s.Equal(uint64(1), histogramSampleCount(s.T(), persistenceDuration))

	spans := s.recorder.Ended()
	s.Require().Len(spans, 1)
	s.Equal("telemetry.persistence.save", spans[0].Name())
}

type stubMeasurementRepository struct {
	err error
}

func (r stubMeasurementRepository) Save(_ context.Context, _ telemetry.Measurement) error {
	return r.err
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
