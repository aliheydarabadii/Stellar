package ports

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandlerHealthzReturnsOK(t *testing.T) {
	t.Parallel()

	handler := NewHealthHandler(func() bool { return true })
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHealthHandlerReadyzReturnsOKWhenReady(t *testing.T) {
	t.Parallel()

	handler := NewHealthHandler(func() bool { return true })
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHealthHandlerReadyzReturnsServiceUnavailableWhenNotReady(t *testing.T) {
	t.Parallel()

	handler := NewHealthHandler(func() bool { return false })
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
