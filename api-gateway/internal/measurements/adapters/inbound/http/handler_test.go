package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	redisadapter "api_gateway/internal/measurements/adapters/outbound/redis"
	getmeasurements "api_gateway/internal/measurements/application/get_measurements"
	"api_gateway/internal/measurements/domain"
	"api_gateway/internal/platform/requestctx"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type HTTPSuite struct {
	suite.Suite
}

func TestHTTPSuite(t *testing.T) {
	suite.Run(t, new(HTTPSuite))
}

func (s *HTTPSuite) TestHandlerGetMeasurementsReturnsJSON() {
	base := s.baseTime()
	handler := newTestHandler(s.T(), &fakeMeasurementsClient{
		series: domain.MeasurementSeries{
			AssetID: "asset-1",
			Points: []domain.MeasurementPoint{
				{
					Timestamp:   base,
					Setpoint:    100,
					ActivePower: 55,
				},
			},
		},
	}, &fakeMeasurementsCache{}, nil)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusOK, rec.Code)

	var got domain.MeasurementSeries
	err := json.Unmarshal(rec.Body.Bytes(), &got)
	s.Require().NoError(err)
	s.Equal("asset-1", got.AssetID)
	s.Len(got.Points, 1)
}

func (s *HTTPSuite) TestHandlerGetMeasurementsRejectsInvalidAssetID() {
	base := s.baseTime()
	handler := newTestHandler(s.T(), &fakeMeasurementsClient{}, &fakeMeasurementsCache{}, nil)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/%20/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusBadRequest, rec.Code)
}

func (s *HTTPSuite) TestHandlerGetMeasurementsRejectsMissingTimes() {
	handler := newTestHandler(s.T(), &fakeMeasurementsClient{}, &fakeMeasurementsCache{}, nil)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/asset-1/measurements", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusBadRequest, rec.Code)
}

func (s *HTTPSuite) TestHandlerGetMeasurementsRejectsInvalidRange() {
	base := s.baseTime()
	handler := newTestHandler(s.T(), &fakeMeasurementsClient{}, &fakeMeasurementsCache{}, nil)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/asset-1/measurements?from="+base.Add(time.Minute).Format(time.RFC3339)+"&to="+base.Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusBadRequest, rec.Code)
}

func (s *HTTPSuite) TestHandlerGetMeasurementsReturnsServiceUnavailable() {
	base := s.baseTime()
	handler := newTestHandler(s.T(), &fakeMeasurementsClient{
		err: getmeasurements.ErrMeasurementServiceUnavailable,
	}, &fakeMeasurementsCache{}, nil)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusServiceUnavailable, rec.Code)
}

func (s *HTTPSuite) TestHandlerGetMeasurementsReturnsBadRequestForDownstreamInvalidArgument() {
	base := s.baseTime()
	handler := newTestHandler(s.T(), &fakeMeasurementsClient{
		err: getmeasurements.NewDownstreamInvalidRequestError("query time range exceeds maximum allowed window"),
	}, &fakeMeasurementsCache{}, nil)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusBadRequest, rec.Code)
	s.Contains(rec.Body.String(), "query time range exceeds maximum allowed window")
}

func (s *HTTPSuite) TestHandlerPropagatesRequestMetadataToContextAndResponse() {
	base := s.baseTime()
	client := &fakeMeasurementsClient{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}
	handler := newTestHandler(s.T(), client, &fakeMeasurementsCache{}, nil)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	req.Header.Set(requestctx.RequestIDHeader, "req-123")
	req.Header.Set(requestctx.CorrelationIDHeader, "corr-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusOK, rec.Code)
	s.Equal("req-123", rec.Header().Get(requestctx.RequestIDHeader))
	s.Equal("corr-123", rec.Header().Get(requestctx.CorrelationIDHeader))
	s.Equal("req-123", client.requestID)
	s.Equal("corr-123", client.correlationID)
}

func (s *HTTPSuite) TestHandlerFallsBackToCorrelationIDWhenRequestIDMissing() {
	base := s.baseTime()
	client := &fakeMeasurementsClient{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}
	handler := newTestHandler(s.T(), client, &fakeMeasurementsCache{}, nil)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	req.Header.Set(requestctx.CorrelationIDHeader, "corr-only")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusOK, rec.Code)
	s.Equal("corr-only", rec.Header().Get(requestctx.RequestIDHeader))
	s.Equal("corr-only", rec.Header().Get(requestctx.CorrelationIDHeader))
	s.Equal("corr-only", client.requestID)
	s.Equal("corr-only", client.correlationID)
}

func (s *HTTPSuite) TestHandlerGeneratesRequestIDWhenMissing() {
	base := s.baseTime()
	client := &fakeMeasurementsClient{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}
	handler := newTestHandler(s.T(), client, &fakeMeasurementsCache{}, nil)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusOK, rec.Code)
	s.NotEmpty(rec.Header().Get(requestctx.RequestIDHeader))
	s.Empty(rec.Header().Get(requestctx.CorrelationIDHeader))
	s.NotEmpty(client.requestID)
	s.Empty(client.correlationID)
}

func (s *HTTPSuite) TestHandlerAccessLogIncludesCacheMissAndRequestMetadata() {
	base := s.baseTime()
	client := &fakeMeasurementsClient{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}
	cache := &fakeMeasurementsCache{}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(io.Discard, logs), nil))
	handler := newTestHandler(s.T(), client, cache, logger)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	req.Header.Set(requestctx.RequestIDHeader, "req-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusOK, rec.Code)

	logOutput := logs.String()
	s.Contains(logOutput, `"msg":"handled http request"`)
	s.Contains(logOutput, `"request_id":"req-123"`)
	s.Contains(logOutput, `"status":200`)
	s.Contains(logOutput, `"cache_status":"miss"`)
}

func (s *HTTPSuite) TestHandlerAccessLogIncludesCacheHit() {
	base := s.baseTime()
	cache := &fakeMeasurementsCache{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
		found:  true,
	}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(io.Discard, logs), nil))
	handler := newTestHandler(s.T(), &fakeMeasurementsClient{}, cache, logger)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusOK, rec.Code)
	s.Contains(logs.String(), `"cache_status":"hit"`)
}

func (s *HTTPSuite) TestHandlerDefaultsCacheStatusWhenQueryHandlerDoesNotSetIt() {
	base := s.baseTime()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(io.Discard, logs), nil))
	handler := NewHandler(fakeQueryHandler{
		series: domain.MeasurementSeries{AssetID: "asset-1"},
	}, logger, 0)

	req := httptest.NewRequest(stdhttp.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusOK, rec.Code)
	s.Contains(logs.String(), `"cache_status":"not_applicable"`)
}

func (s *HTTPSuite) TestHandlerCachesIdenticalRequestsWithinTTL() {
	redisServer := miniredis.RunT(s.T())
	cache, err := redisadapter.NewCache(context.Background(), redisServer.Addr(), "", "", 0)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		if closeErr := cache.Close(); closeErr != nil {
			s.T().Errorf("close redis cache: %v", closeErr)
		}
	})

	base := s.baseTime()
	client := &countingMeasurementsClient{
		series: domain.MeasurementSeries{
			AssetID: "asset-1",
			Points: []domain.MeasurementPoint{
				{
					Timestamp:   base,
					Setpoint:    42,
					ActivePower: 41,
				},
			},
		},
	}
	handler := newTestHandler(s.T(), client, cache, nil)

	server := httptest.NewServer(handler)
	s.T().Cleanup(server.Close)

	url := server.URL + "/assets/asset-1/measurements?from=" + base.Format(time.RFC3339) + "&to=" + base.Add(time.Minute).Format(time.RFC3339)

	for i := 0; i < 2; i++ {
		resp, requestErr := stdhttp.Get(url)
		s.Require().NoError(requestErr)
		s.Equal(stdhttp.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	}

	s.Equal(1, client.calls)
}

func (s *HTTPSuite) TestHealthHandlerHealthzAlwaysOK() {
	handler := NewHealthHandler(func(context.Context) error {
		return errors.New("should not be called")
	}, time.Second)

	req := httptest.NewRequest(stdhttp.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusOK, rec.Code)
}

func (s *HTTPSuite) TestHealthHandlerReadyzReturnsOKWhenProbeSucceeds() {
	handler := NewHealthHandler(func(context.Context) error {
		return nil
	}, time.Second)

	req := httptest.NewRequest(stdhttp.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusOK, rec.Code)
}

func (s *HTTPSuite) TestHealthHandlerReadyzReturnsUnavailableWhenProbeFails() {
	handler := NewHealthHandler(func(context.Context) error {
		return errors.New("dependency unavailable")
	}, time.Second)

	req := httptest.NewRequest(stdhttp.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusServiceUnavailable, rec.Code)
}

func (s *HTTPSuite) TestHealthHandlerReadyzRespectsProbeTimeout() {
	handler := NewHealthHandler(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}, 10*time.Millisecond)

	req := httptest.NewRequest(stdhttp.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	s.Equal(stdhttp.StatusServiceUnavailable, rec.Code)
}

func (s *HTTPSuite) baseTime() time.Time {
	return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
}

func newTestHandler(t *testing.T, reader getmeasurements.MeasurementsReader, cache getmeasurements.MeasurementsCache, logger *slog.Logger) stdhttp.Handler {
	t.Helper()

	cachedReader, err := redisadapter.NewCachedReader(
		reader,
		cache,
		5*time.Minute,
		redisadapter.MeasurementsKey,
		nil,
	)
	require.NoError(t, err)

	useCase, err := getmeasurements.NewUseCase(cachedReader)
	require.NoError(t, err)

	return NewHandler(useCase, logger, 0)
}

type fakeQueryHandler struct {
	series domain.MeasurementSeries
	err    error
}

func (h fakeQueryHandler) Handle(_ context.Context, qry getmeasurements.Query) (domain.MeasurementSeries, error) {
	if h.err != nil {
		return domain.MeasurementSeries{}, h.err
	}

	if h.series.AssetID == "" {
		h.series.AssetID = qry.AssetID
	}

	return h.series, nil
}

type fakeMeasurementsClient struct {
	series        domain.MeasurementSeries
	err           error
	requestID     string
	correlationID string
}

func (f *fakeMeasurementsClient) GetMeasurements(ctx context.Context, assetID string, from, to time.Time) (domain.MeasurementSeries, error) {
	f.requestID = requestctx.RequestIDFromContext(ctx)
	f.correlationID = requestctx.CorrelationIDFromContext(ctx)

	if f.err != nil {
		return domain.MeasurementSeries{}, f.err
	}

	if f.series.AssetID == "" {
		f.series.AssetID = assetID
	}

	return f.series, nil
}

type fakeMeasurementsCache struct {
	series domain.MeasurementSeries
	found  bool
	err    error
}

func (f *fakeMeasurementsCache) Get(_ context.Context, _ string) (domain.MeasurementSeries, bool, error) {
	if f.err != nil {
		return domain.MeasurementSeries{}, false, f.err
	}

	return f.series, f.found, nil
}

func (f *fakeMeasurementsCache) Set(_ context.Context, _ string, value domain.MeasurementSeries, _ time.Duration) error {
	if f.err != nil {
		return f.err
	}

	f.series = value
	f.found = true
	return nil
}

type countingMeasurementsClient struct {
	calls  int
	series domain.MeasurementSeries
}

func (c *countingMeasurementsClient) GetMeasurements(_ context.Context, assetID string, from, to time.Time) (domain.MeasurementSeries, error) {
	c.calls++
	if c.series.AssetID == "" {
		c.series.AssetID = assetID
	}

	return c.series, nil
}
