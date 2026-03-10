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
)

func TestTickerWorkerStartCreatesCommandWithTimestamp(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

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

	worker, err := NewTickerWorker(5*time.Millisecond, handler, logger)
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
}

func TestTickerWorkerStartSurvivesHandlerErrors(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	callCh := make(chan struct{}, 4)
	handler := &stubCollectTelemetryHandler{
		handleFn: func(_ context.Context, _ command.CollectTelemetry) error {
			callCh <- struct{}{}
			if len(callCh) >= 2 {
				cancel()
			}

			return errors.New("handler failed")
		},
	}

	worker, err := NewTickerWorker(5*time.Millisecond, handler, logger)
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
}

type stubCollectTelemetryHandler struct {
	handleFn func(ctx context.Context, cmd command.CollectTelemetry) error
}

func (h *stubCollectTelemetryHandler) Handle(ctx context.Context, cmd command.CollectTelemetry) error {
	return h.handleFn(ctx, cmd)
}
