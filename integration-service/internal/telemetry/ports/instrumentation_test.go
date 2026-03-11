package ports

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"stellar/internal/telemetry/app/command"
	"stellar/internal/telemetry/domain"
)

type InstrumentationTestSuite struct {
	suite.Suite
	metrics  *Metrics
	tracer   trace.Tracer
	recorder *tracetest.SpanRecorder
}

func TestInstrumentationTestSuite(t *testing.T) {
	suite.Run(t, new(InstrumentationTestSuite))
}

func (s *InstrumentationTestSuite) SetupTest() {
	s.metrics = NewMetrics()
	s.tracer, s.recorder = newTestTracer()
}

func (s *InstrumentationTestSuite) TestInstrumentTelemetrySourceObservesReadDuration() {
	source := InstrumentTelemetrySource(stubTelemetrySource{
		reading: command.TelemetryReading{
			Setpoint:    100,
			ActivePower: 50,
		},
	}, s.metrics, s.tracer)

	_, err := source.Read(context.Background())
	s.Require().NoError(err)

	s.Equal(uint64(1), histogramSampleCount(s.T(), s.metrics.sourceReadDuration))

	spans := s.recorder.Ended()
	s.Require().Len(spans, 1)
	s.Equal("telemetry.source.read", spans[0].Name())
}

func (s *InstrumentationTestSuite) TestInstrumentMeasurementRepositoryObservesPersistenceDuration() {
	repository := InstrumentMeasurementRepository(stubMeasurementRepository{}, s.metrics, s.tracer)

	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 50, time.Now().UTC())
	s.Require().NoError(err)

	s.Require().NoError(repository.Save(context.Background(), measurement))
	s.Equal(uint64(1), histogramSampleCount(s.T(), s.metrics.persistenceDuration))

	spans := s.recorder.Ended()
	s.Require().Len(spans, 1)
	s.Equal("telemetry.persistence.save", spans[0].Name())
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
