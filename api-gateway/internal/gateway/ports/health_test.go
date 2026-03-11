package ports

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthHandlerHealthzAlwaysOK(t *testing.T) {
	t.Parallel()

	handler := NewHealthHandler(func(context.Context) error {
		return errors.New("should not be called")
	}, time.Second)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestHealthHandlerReadyzReturnsOKWhenProbeSucceeds(t *testing.T) {
	t.Parallel()

	handler := NewHealthHandler(func(context.Context) error {
		return nil
	}, time.Second)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestHealthHandlerReadyzReturnsUnavailableWhenProbeFails(t *testing.T) {
	t.Parallel()

	handler := NewHealthHandler(func(context.Context) error {
		return errors.New("dependency unavailable")
	}, time.Second)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestHealthHandlerReadyzRespectsProbeTimeout(t *testing.T) {
	t.Parallel()

	handler := NewHealthHandler(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}, 10*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}
