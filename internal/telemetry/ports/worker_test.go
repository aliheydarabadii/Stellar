package ports

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"stellar/internal/telemetry/app/command"
)

func TestTickerWorkerStartCreatesCommandWithTimestamp(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := NewMetrics()

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

	worker, err := NewTickerWorker(5*time.Millisecond, handler, logger, metrics, nil)
	if err != nil {
		t.Fatalf("expected worker to be created, got %v", err)
	}

	before := time.Now().UTC()

	done := make(chan error, 1)
	go func() {
		done <- worker.Start(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected worker to stop cleanly, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for worker to stop")
	}

	after := time.Now().UTC()

	mu.Lock()
	got := received
	mu.Unlock()

	if got.CollectedAt.IsZero() {
		t.Fatal("expected collected timestamp to be set")
	}

	if got.CollectedAt.Location() != time.UTC {
		t.Fatalf("expected UTC timestamp, got location %v", got.CollectedAt.Location())
	}

	if got.CollectedAt.Before(before) || got.CollectedAt.After(after) {
		t.Fatalf("expected collected timestamp between %v and %v, got %v", before, after, got.CollectedAt)
	}

	if got := testutil.ToFloat64(metrics.attemptsCounter); got != 1 {
		t.Fatalf("expected 1 attempt, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.successesCounter); got != 1 {
		t.Fatalf("expected 1 success, got %v", got)
	}
	wantUnix := float64(got.CollectedAt.Unix())
	if got := testutil.ToFloat64(metrics.lastAttemptGauge); got != wantUnix {
		t.Fatalf("expected last attempt gauge %v, got %v", wantUnix, got)
	}
	if got := testutil.ToFloat64(metrics.lastSuccessGauge); got != wantUnix {
		t.Fatalf("expected last success gauge %v, got %v", wantUnix, got)
	}
	if got := histogramSampleCount(t, metrics.collectionDuration); got != 1 {
		t.Fatalf("expected 1 collection duration sample, got %d", got)
	}
}

func TestTickerWorkerStartSurvivesHandlerErrors(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := NewMetrics()

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

	worker, err := NewTickerWorker(5*time.Millisecond, handler, logger, metrics, nil)
	if err != nil {
		t.Fatalf("expected worker to be created, got %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- worker.Start(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected worker to stop cleanly, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for worker to stop")
	}

	if got := len(callCh); got < 2 {
		t.Fatalf("expected handler to be called at least twice despite errors, got %d", got)
	}

	if got := testutil.ToFloat64(metrics.failuresCounter); got < 2 {
		t.Fatalf("expected at least 2 failures, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.sourceFailuresCounter); got < 2 {
		t.Fatalf("expected at least 2 source failures, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.persistenceFailuresCounter); got != 0 {
		t.Fatalf("expected 0 persistence failures, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.successesCounter); got != 0 {
		t.Fatalf("expected 0 successes, got %v", got)
	}
	if got := histogramSampleCount(t, metrics.collectionDuration); got < 2 {
		t.Fatalf("expected at least 2 collection duration samples, got %d", got)
	}
}

func TestTickerWorkerStartCreatesTraceSpan(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := NewMetrics()
	tracer, recorder := newTestTracer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &stubCollectTelemetryHandler{
		handleFn: func(_ context.Context, _ command.CollectTelemetry) error {
			cancel()
			return nil
		},
	}

	worker, err := NewTickerWorker(5*time.Millisecond, handler, logger, metrics, tracer)
	if err != nil {
		t.Fatalf("expected worker to be created, got %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- worker.Start(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected worker to stop cleanly, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for worker to stop")
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "telemetry.collect" {
		t.Fatalf("expected span name %q, got %q", "telemetry.collect", spans[0].Name())
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
