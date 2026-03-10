package ports

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServerExposesMetricsEndpoint(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics()
	collectedAt := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	metrics.RecordAttempt(collectedAt)
	metrics.RecordSuccess(collectedAt)
	metrics.RecordValidationFailure()
	metrics.RecordFailure()
	metrics.RecordSourceFailure()
	metrics.RecordPersistenceFailure()

	server, err := NewHTTPServer(":8080", slog.New(slog.NewTextHandler(io.Discard, nil)), metrics)
	if err != nil {
		t.Fatalf("expected http server to be created, got %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	server.newMux().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain; version=0.0.4") {
		t.Fatalf("expected metrics content type, got %q", got)
	}

	body := recorder.Body.String()
	assertContains(t, body, "integration_service_telemetry_collection_attempts_total")
	assertContains(t, body, "integration_service_telemetry_collection_success_total")
	assertContains(t, body, "integration_service_telemetry_collection_validation_failures_total")
	assertContains(t, body, "integration_service_telemetry_collection_failures_total")
	assertContains(t, body, "integration_service_telemetry_source_failures_total")
	assertContains(t, body, "integration_service_telemetry_persistence_failures_total")
	assertContains(t, body, "integration_service_telemetry_collection_duration_seconds")
	assertContains(t, body, "integration_service_telemetry_source_read_duration_seconds")
	assertContains(t, body, "integration_service_telemetry_persistence_duration_seconds")
	assertContains(t, body, "integration_service_telemetry_last_attempt_timestamp_seconds")
	assertContains(t, body, "integration_service_telemetry_last_success_timestamp_seconds")
	assertContains(t, body, "go_gc_duration_seconds")
}

func assertContains(t *testing.T, body, want string) {
	t.Helper()

	if !strings.Contains(body, want) {
		t.Fatalf("expected body to contain %q, got %q", want, body)
	}
}
