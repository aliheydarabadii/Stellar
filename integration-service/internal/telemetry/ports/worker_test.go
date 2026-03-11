package ports

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"stellar/internal/telemetry/app/command"

	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/suite"
)

type TickerWorkerTestSuite struct {
	suite.Suite
	logger    *slog.Logger
	metrics   *Metrics
	readiness *Readiness
}

func TestTickerWorkerTestSuite(t *testing.T) {
	suite.Run(t, new(TickerWorkerTestSuite))
}

func (s *TickerWorkerTestSuite) SetupTest() {
	s.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	s.metrics = NewMetrics()

	readiness, err := NewReadiness(time.Minute)
	s.Require().NoError(err)
	s.readiness = readiness
}

func (s *TickerWorkerTestSuite) TestTickerWorkerStartCreatesCommandWithTimestamp() {
	var (
		mu       sync.Mutex
		received command.CollectTelemetry
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &stubCollectTelemetryHandler{
		handleFn: func(_ context.Context, cmd command.CollectTelemetry) error {
			mu.Lock()
			received = cmd
			mu.Unlock()
			cancel()

			return nil
		},
	}

	worker, err := NewTickerWorker(5*time.Millisecond, handler, s.logger, s.metrics, s.readiness, nil)
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

	s.Assert().Equal(float64(1), testutil.ToFloat64(s.metrics.attemptsCounter))
	s.Assert().Equal(float64(1), testutil.ToFloat64(s.metrics.successesCounter))

	wantUnix := float64(got.CollectedAt.Unix())
	s.Assert().Equal(wantUnix, testutil.ToFloat64(s.metrics.lastAttemptGauge))
	s.Assert().Equal(wantUnix, testutil.ToFloat64(s.metrics.lastSuccessGauge))
	s.Assert().Equal(uint64(1), histogramSampleCount(s.T(), s.metrics.collectionDuration))
	s.Assert().True(s.readiness.Ready(time.Now().UTC()))
}

func (s *TickerWorkerTestSuite) TestTickerWorkerStartSurvivesHandlerErrors() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	callCh := make(chan struct{}, 4)
	handler := &stubCollectTelemetryHandler{
		handleFn: func(_ context.Context, _ command.CollectTelemetry) error {
			callCh <- struct{}{}
			if len(callCh) >= 2 {
				cancel()
			}

			return errors.Join(command.ErrTelemetrySource, errors.New("handler failed"))
		},
	}

	worker, err := NewTickerWorker(5*time.Millisecond, handler, s.logger, s.metrics, s.readiness, nil)
	s.Require().NoError(err)

	s.runWorker(ctx, worker)

	s.Assert().GreaterOrEqual(len(callCh), 2)
	s.Assert().GreaterOrEqual(testutil.ToFloat64(s.metrics.failuresCounter), float64(2))
	s.Assert().GreaterOrEqual(testutil.ToFloat64(s.metrics.sourceFailuresCounter), float64(2))
	s.Assert().Equal(float64(0), testutil.ToFloat64(s.metrics.persistenceFailuresCounter))
	s.Assert().Equal(float64(0), testutil.ToFloat64(s.metrics.successesCounter))
	s.Assert().GreaterOrEqual(histogramSampleCount(s.T(), s.metrics.collectionDuration), uint64(2))
	s.Assert().False(s.readiness.Ready(time.Now().UTC()))
}

func (s *TickerWorkerTestSuite) TestTickerWorkerStartCreatesTraceSpan() {
	tracer, recorder := newTestTracer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &stubCollectTelemetryHandler{
		handleFn: func(_ context.Context, _ command.CollectTelemetry) error {
			cancel()
			return nil
		},
	}

	worker, err := NewTickerWorker(5*time.Millisecond, handler, s.logger, s.metrics, s.readiness, tracer)
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
	handleFn func(ctx context.Context, cmd command.CollectTelemetry) error
}

func (h *stubCollectTelemetryHandler) Handle(ctx context.Context, cmd command.CollectTelemetry) error {
	return h.handleFn(ctx, cmd)
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
