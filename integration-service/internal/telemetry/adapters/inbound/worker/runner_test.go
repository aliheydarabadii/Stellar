package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	healthplatform "stellar/internal/platform/health"
	metricsplatform "stellar/internal/platform/metrics"
	collecttelemetry "stellar/internal/telemetry/application/collect_telemetry"

	"github.com/stretchr/testify/suite"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type TickerWorkerTestSuite struct {
	suite.Suite
	logger    *slog.Logger
	metrics   *metricsplatform.Metrics
	readiness *healthplatform.Readiness
}

func TestTickerWorkerTestSuite(t *testing.T) {
	suite.Run(t, new(TickerWorkerTestSuite))
}

func (s *TickerWorkerTestSuite) SetupTest() {
	s.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	s.metrics = metricsplatform.NewMetrics()

	readiness, err := healthplatform.NewReadiness(time.Minute)
	s.Require().NoError(err)
	s.readiness = readiness
}

func (s *TickerWorkerTestSuite) TestTickerWorkerStartCreatesCommandWithTimestamp() {
	var (
		mu       sync.Mutex
		received collecttelemetry.CollectTelemetry
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &stubCollectTelemetryHandler{
		handleFn: func(_ context.Context, cmd collecttelemetry.CollectTelemetry) error {
			mu.Lock()
			received = cmd
			mu.Unlock()
			cancel()

			return nil
		},
	}

	worker, err := NewRunner(5*time.Millisecond, handler, s.logger, s.metrics, s.readiness, nil)
	s.Require().NoError(err)

	before := time.Now().UTC()
	s.runWorker(ctx, worker)
	after := time.Now().UTC()

	mu.Lock()
	got := received
	mu.Unlock()

	s.Assert().False(got.CollectedAt.IsZero())
	s.Assert().Equal(time.UTC, got.CollectedAt.Location())
	s.Assert().False(got.CollectedAt.Before(before))
	s.Assert().False(got.CollectedAt.After(after))

	body := s.scrapeMetrics()
	s.Assert().Equal(float64(1), metricValue(s.T(), body, "integration_service_telemetry_collection_attempts_total"))
	s.Assert().Equal(float64(1), metricValue(s.T(), body, "integration_service_telemetry_collection_success_total"))

	wantUnix := float64(got.CollectedAt.Unix())
	s.Assert().Equal(wantUnix, metricValue(s.T(), body, "integration_service_telemetry_last_attempt_timestamp_seconds"))
	s.Assert().Equal(wantUnix, metricValue(s.T(), body, "integration_service_telemetry_last_success_timestamp_seconds"))
	s.Assert().Equal(float64(1), metricValue(s.T(), body, "integration_service_telemetry_collection_duration_seconds_count"))
	s.Assert().True(s.readiness.Ready(time.Now().UTC()))
}

func (s *TickerWorkerTestSuite) TestTickerWorkerStartSurvivesHandlerErrors() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	callCh := make(chan struct{}, 4)
	handler := &stubCollectTelemetryHandler{
		handleFn: func(_ context.Context, _ collecttelemetry.CollectTelemetry) error {
			callCh <- struct{}{}
			if len(callCh) >= 2 {
				cancel()
			}

			return errors.Join(collecttelemetry.ErrTelemetrySource, errors.New("handler failed"))
		},
	}

	worker, err := NewRunner(5*time.Millisecond, handler, s.logger, s.metrics, s.readiness, nil)
	s.Require().NoError(err)

	s.runWorker(ctx, worker)

	body := s.scrapeMetrics()
	s.Assert().GreaterOrEqual(len(callCh), 2)
	s.Assert().GreaterOrEqual(metricValue(s.T(), body, "integration_service_telemetry_collection_failures_total"), float64(2))
	s.Assert().GreaterOrEqual(metricValue(s.T(), body, "integration_service_telemetry_source_failures_total"), float64(2))
	s.Assert().Equal(float64(0), metricValue(s.T(), body, "integration_service_telemetry_persistence_failures_total"))
	s.Assert().Equal(float64(0), metricValue(s.T(), body, "integration_service_telemetry_collection_success_total"))
	s.Assert().GreaterOrEqual(metricValue(s.T(), body, "integration_service_telemetry_collection_duration_seconds_count"), float64(2))
	s.Assert().False(s.readiness.Ready(time.Now().UTC()))
}

func (s *TickerWorkerTestSuite) TestTickerWorkerStartCreatesTraceSpan() {
	tracer, recorder := newTestTracer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &stubCollectTelemetryHandler{
		handleFn: func(_ context.Context, _ collecttelemetry.CollectTelemetry) error {
			cancel()
			return nil
		},
	}

	worker, err := NewRunner(5*time.Millisecond, handler, s.logger, s.metrics, s.readiness, tracer)
	s.Require().NoError(err)

	s.runWorker(ctx, worker)

	spans := recorder.Ended()
	s.Require().Len(spans, 1)
	s.Assert().Equal("telemetry.collect", spans[0].Name())
}

func (s *TickerWorkerTestSuite) runWorker(ctx context.Context, worker *TickerWorker) {
	s.T().Helper()

	done := make(chan error, 1)
	go func() {
		done <- worker.Start(ctx)
	}()

	select {
	case err := <-done:
		s.Require().NoError(err)
	case <-time.After(250 * time.Millisecond):
		s.T().Fatal("timed out waiting for worker to stop")
	}
}

type stubCollectTelemetryHandler struct {
	handleFn func(ctx context.Context, cmd collecttelemetry.CollectTelemetry) error
}

func (h *stubCollectTelemetryHandler) Handle(ctx context.Context, cmd collecttelemetry.CollectTelemetry) error {
	return h.handleFn(ctx, cmd)
}

func (s *TickerWorkerTestSuite) scrapeMetrics() string {
	s.T().Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	s.metrics.ServeHTTP(recorder, request)

	return recorder.Body.String()
}

func metricValue(t *testing.T, body, name string) float64 {
	t.Helper()

	for _, line := range strings.Split(body, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != name {
			continue
		}

		value, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.Fatalf("parse metric %s: %v", name, err)
		}

		return value
	}

	t.Fatalf("metric %s not found", name)
	return 0
}

func newTestTracer() (trace.Tracer, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider()
	provider.RegisterSpanProcessor(recorder)

	return provider.Tracer("test"), recorder
}
