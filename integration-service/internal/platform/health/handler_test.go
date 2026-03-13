package health

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/suite"
	influxdbadapter "stellar/internal/telemetry/adapters/outbound/influxdb"
	modbusadapter "stellar/internal/telemetry/adapters/outbound/modbus"
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
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	modbusadapter.MustRegisterMetrics(registry)
	influxdbadapter.MustRegisterMetrics(registry)
	readiness, err := NewReadiness(time.Minute)
	s.Require().NoError(err)

	server, err := NewServer(":8080", s.logger, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}), readiness)
	s.Require().NoError(err)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	server.newMux().ServeHTTP(recorder, request)

	s.Assert().Equal(http.StatusOK, recorder.Code)
	s.Assert().True(strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/plain; version=0.0.4"))

	body := recorder.Body.String()
	s.assertContains(body, "integration_service_telemetry_source_read_duration_seconds")
	s.assertContains(body, "integration_service_telemetry_persistence_duration_seconds")
	s.assertContains(body, "go_gc_duration_seconds")
}

func (s *HTTPServerTestSuite) TestServerReadyzRequiresRecentSuccessfulCollection() {
	readiness, err := NewReadiness(time.Minute)
	s.Require().NoError(err)

	server, err := NewServer(":8080", s.logger, http.NotFoundHandler(), readiness)
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

	server, err := NewServer(":8080", s.logger, http.NotFoundHandler(), readiness)
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
