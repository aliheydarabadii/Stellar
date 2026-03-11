package ports

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type HTTPServerTestSuite struct {
	suite.Suite
	logger *slog.Logger
}

func TestHTTPServerTestSuite(t *testing.T) {
	suite.Run(t, new(HTTPServerTestSuite))
}

func (s *HTTPServerTestSuite) SetupTest() {
	s.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
}

func (s *HTTPServerTestSuite) TestServerExposesMetricsEndpoint() {
	metrics := NewMetrics()
	readiness, err := NewReadiness(time.Minute)
	s.Require().NoError(err)

	collectedAt := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	metrics.RecordAttempt(collectedAt)
	metrics.RecordSuccess(collectedAt)
	readiness.MarkSuccess(collectedAt)
	metrics.RecordValidationFailure()
	metrics.RecordFailure()
	metrics.RecordSourceFailure()
	metrics.RecordPersistenceFailure()

	server, err := NewHTTPServer(":8080", s.logger, metrics, readiness)
	s.Require().NoError(err)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	server.newMux().ServeHTTP(recorder, request)

	s.Assert().Equal(http.StatusOK, recorder.Code)
	s.Assert().True(strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/plain; version=0.0.4"))

	body := recorder.Body.String()
	s.assertContains(body, "integration_service_telemetry_collection_attempts_total")
	s.assertContains(body, "integration_service_telemetry_collection_success_total")
	s.assertContains(body, "integration_service_telemetry_collection_validation_failures_total")
	s.assertContains(body, "integration_service_telemetry_collection_failures_total")
	s.assertContains(body, "integration_service_telemetry_source_failures_total")
	s.assertContains(body, "integration_service_telemetry_persistence_failures_total")
	s.assertContains(body, "integration_service_telemetry_collection_duration_seconds")
	s.assertContains(body, "integration_service_telemetry_source_read_duration_seconds")
	s.assertContains(body, "integration_service_telemetry_persistence_duration_seconds")
	s.assertContains(body, "integration_service_telemetry_last_attempt_timestamp_seconds")
	s.assertContains(body, "integration_service_telemetry_last_success_timestamp_seconds")
	s.assertContains(body, "go_gc_duration_seconds")
}

func (s *HTTPServerTestSuite) TestServerReadyzRequiresRecentSuccessfulCollection() {
	readiness, err := NewReadiness(time.Minute)
	s.Require().NoError(err)

	server, err := NewHTTPServer(":8080", s.logger, NewMetrics(), readiness)
	s.Require().NoError(err)

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	notReadyRecorder := httptest.NewRecorder()
	server.newMux().ServeHTTP(notReadyRecorder, request)
	s.Assert().Equal(http.StatusServiceUnavailable, notReadyRecorder.Code)

	readiness.MarkSuccess(time.Now().UTC())

	readyRecorder := httptest.NewRecorder()
	server.newMux().ServeHTTP(readyRecorder, request)
	s.Assert().Equal(http.StatusOK, readyRecorder.Code)
}

func (s *HTTPServerTestSuite) TestServerReadyzFailsWhenSuccessIsStale() {
	readiness, err := NewReadiness(100 * time.Millisecond)
	s.Require().NoError(err)
	readiness.MarkSuccess(time.Now().UTC().Add(-time.Second))

	server, err := NewHTTPServer(":8080", s.logger, NewMetrics(), readiness)
	s.Require().NoError(err)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	server.newMux().ServeHTTP(recorder, request)
	s.Assert().Equal(http.StatusServiceUnavailable, recorder.Code)
}

func (s *HTTPServerTestSuite) assertContains(body, want string) {
	s.T().Helper()
	s.Assert().Contains(body, want)
}
