package ports

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"api_gateway/internal/gateway/app"
	"api_gateway/internal/gateway/app/query"
	"api_gateway/internal/gateway/requestctx"
)

func TestHTTPHandlerGetMeasurementsReturnsJSON(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	handler := newTestHTTPHandler(t, &fakeMeasurementsClient{
		series: query.MeasurementSeries{
			AssetID: "asset-1",
			Points: []query.MeasurementPoint{
				{
					Timestamp:   base,
					Setpoint:    100,
					ActivePower: 55,
				},
			},
		},
	}, &fakeMeasurementsCache{})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var got query.MeasurementSeries
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AssetID != "asset-1" || len(got.Points) != 1 {
		t.Fatalf("unexpected response %+v", got)
	}
}

func TestHTTPHandlerGetMeasurementsRejectsInvalidAssetID(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	handler := newTestHTTPHandler(t, &fakeMeasurementsClient{}, &fakeMeasurementsCache{})

	req := httptest.NewRequest(http.MethodGet, "/assets/%20/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestHTTPHandlerGetMeasurementsRejectsMissingTimes(t *testing.T) {
	t.Parallel()

	handler := newTestHTTPHandler(t, &fakeMeasurementsClient{}, &fakeMeasurementsCache{})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset-1/measurements", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestHTTPHandlerGetMeasurementsRejectsInvalidRange(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	handler := newTestHTTPHandler(t, &fakeMeasurementsClient{}, &fakeMeasurementsCache{})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset-1/measurements?from="+base.Add(time.Minute).Format(time.RFC3339)+"&to="+base.Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestHTTPHandlerGetMeasurementsReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	handler := newTestHTTPHandler(t, &fakeMeasurementsClient{
		err: query.ErrMeasurementServiceUnavailable,
	}, &fakeMeasurementsCache{})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestHTTPHandlerGetMeasurementsReturnsBadRequestForDownstreamInvalidArgument(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	handler := newTestHTTPHandler(t, &fakeMeasurementsClient{
		err: query.NewDownstreamInvalidRequestError("query time range exceeds maximum allowed window"),
	}, &fakeMeasurementsCache{})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "query time range exceeds maximum allowed window") {
		t.Fatalf("expected downstream validation message, got %s", rec.Body.String())
	}
}

func TestHTTPHandlerPropagatesRequestMetadataToContextAndResponse(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	client := &fakeMeasurementsClient{
		series: query.MeasurementSeries{AssetID: "asset-1"},
	}
	handler := newTestHTTPHandler(t, client, &fakeMeasurementsCache{})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	req.Header.Set(requestctx.RequestIDHeader, "req-123")
	req.Header.Set(requestctx.CorrelationIDHeader, "corr-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get(requestctx.RequestIDHeader); got != "req-123" {
		t.Fatalf("expected response request id %q, got %q", "req-123", got)
	}
	if got := rec.Header().Get(requestctx.CorrelationIDHeader); got != "corr-123" {
		t.Fatalf("expected response correlation id %q, got %q", "corr-123", got)
	}
	if client.requestID != "req-123" {
		t.Fatalf("expected client request id %q, got %q", "req-123", client.requestID)
	}
	if client.correlationID != "corr-123" {
		t.Fatalf("expected client correlation id %q, got %q", "corr-123", client.correlationID)
	}
}

func TestHTTPHandlerFallsBackToCorrelationIDWhenRequestIDMissing(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	client := &fakeMeasurementsClient{
		series: query.MeasurementSeries{AssetID: "asset-1"},
	}
	handler := newTestHTTPHandler(t, client, &fakeMeasurementsCache{})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	req.Header.Set(requestctx.CorrelationIDHeader, "corr-only")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get(requestctx.RequestIDHeader); got != "corr-only" {
		t.Fatalf("expected response request id %q, got %q", "corr-only", got)
	}
	if got := rec.Header().Get(requestctx.CorrelationIDHeader); got != "corr-only" {
		t.Fatalf("expected response correlation id %q, got %q", "corr-only", got)
	}
	if client.requestID != "corr-only" {
		t.Fatalf("expected client request id %q, got %q", "corr-only", client.requestID)
	}
	if client.correlationID != "corr-only" {
		t.Fatalf("expected client correlation id %q, got %q", "corr-only", client.correlationID)
	}
}

func TestHTTPHandlerGeneratesRequestIDWhenMissing(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	client := &fakeMeasurementsClient{
		series: query.MeasurementSeries{AssetID: "asset-1"},
	}
	handler := newTestHTTPHandler(t, client, &fakeMeasurementsCache{})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get(requestctx.RequestIDHeader); got == "" {
		t.Fatal("expected generated request id")
	}
	if got := rec.Header().Get(requestctx.CorrelationIDHeader); got != "" {
		t.Fatalf("expected empty correlation id, got %q", got)
	}
	if client.requestID == "" {
		t.Fatal("expected client request id to be set")
	}
	if client.correlationID != "" {
		t.Fatalf("expected empty client correlation id, got %q", client.correlationID)
	}
}

func TestHTTPHandlerAccessLogIncludesCacheMissAndRequestMetadata(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	client := &fakeMeasurementsClient{
		series: query.MeasurementSeries{AssetID: "asset-1"},
	}
	cache := &fakeMeasurementsCache{}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(io.Discard, logs), nil))
	handler := newTestHTTPHandlerWithLogger(t, client, cache, logger)

	req := httptest.NewRequest(http.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	req.Header.Set(requestctx.RequestIDHeader, "req-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	logOutput := logs.String()
	if !strings.Contains(logOutput, `"msg":"handled http request"`) {
		t.Fatalf("expected access log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, `"request_id":"req-123"`) {
		t.Fatalf("expected request id in access log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, `"status":200`) {
		t.Fatalf("expected status in access log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, `"cache_status":"miss"`) {
		t.Fatalf("expected cache miss in access log, got %s", logOutput)
	}
}

func TestHTTPHandlerAccessLogIncludesCacheHit(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	cache := &fakeMeasurementsCache{
		series: query.MeasurementSeries{AssetID: "asset-1"},
		found:  true,
	}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(io.Discard, logs), nil))
	handler := newTestHTTPHandlerWithLogger(t, &fakeMeasurementsClient{}, cache, logger)

	req := httptest.NewRequest(http.MethodGet, "/assets/asset-1/measurements?from="+base.Format(time.RFC3339)+"&to="+base.Add(time.Minute).Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(logs.String(), `"cache_status":"hit"`) {
		t.Fatalf("expected cache hit in access log, got %s", logs.String())
	}
}

func newTestHTTPHandler(t *testing.T, client query.MeasurementsClient, cache query.MeasurementsCache) http.Handler {
	return newTestHTTPHandlerWithLogger(t, client, cache, nil)
}

func newTestHTTPHandlerWithLogger(t *testing.T, client query.MeasurementsClient, cache query.MeasurementsCache, logger *slog.Logger) http.Handler {
	t.Helper()

	application, err := app.New(
		client,
		cache,
		5*time.Minute,
		func(assetID string, from, to time.Time) string {
			return assetID + "|" + from.Format(time.RFC3339Nano) + "|" + to.Format(time.RFC3339Nano)
		},
		logger,
	)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	return NewHTTPHandler(application, logger, 0)
}

type fakeMeasurementsClient struct {
	series        query.MeasurementSeries
	err           error
	requestID     string
	correlationID string
}

func (f *fakeMeasurementsClient) GetMeasurements(ctx context.Context, assetID string, from, to time.Time) (query.MeasurementSeries, error) {
	f.requestID = requestctx.RequestIDFromContext(ctx)
	f.correlationID = requestctx.CorrelationIDFromContext(ctx)

	if f.err != nil {
		return query.MeasurementSeries{}, f.err
	}

	if f.series.AssetID == "" {
		f.series.AssetID = assetID
	}

	return f.series, nil
}

type fakeMeasurementsCache struct {
	series query.MeasurementSeries
	found  bool
	err    error
}

func (f *fakeMeasurementsCache) Get(_ context.Context, _ string) (query.MeasurementSeries, bool, error) {
	if f.err != nil {
		return query.MeasurementSeries{}, false, f.err
	}

	return f.series, f.found, nil
}

func (f *fakeMeasurementsCache) Set(_ context.Context, _ string, value query.MeasurementSeries, _ time.Duration) error {
	if f.err != nil {
		return f.err
	}

	f.series = value
	f.found = true
	return nil
}
