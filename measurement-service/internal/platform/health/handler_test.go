package health

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type HealthHandlerSuite struct {
	suite.Suite
}

func TestHealthHandlerSuite(t *testing.T) {
	suite.Run(t, new(HealthHandlerSuite))
}

func (s *HealthHandlerSuite) TestHealthzReturnsOK() {
	handler := NewHealthHandler(func() bool { return true })
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
}

func (s *HealthHandlerSuite) TestReadyzReturnsOKWhenReady() {
	handler := NewHealthHandler(func() bool { return true })
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
}

func (s *HealthHandlerSuite) TestReadyzReturnsServiceUnavailableWhenNotReady() {
	handler := NewHealthHandler(func() bool { return false })
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(http.StatusServiceUnavailable, rec.Code)
}
