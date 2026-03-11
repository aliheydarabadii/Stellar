package ports

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"api_gateway/internal/gateway/app"
	"api_gateway/internal/gateway/app/query"
	"api_gateway/internal/gateway/requestctx"
)

type errorResponse struct {
	Error string `json:"error"`
}

func NewHTTPHandler(application app.Application, logger *slog.Logger, requestTimeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /assets/{asset_id}/measurements", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if requestTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, requestTimeout)
			defer cancel()
		}

		from, err := parseTimeParam(r, "from")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		to, err := parseTimeParam(r, "to")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		series, err := application.Queries.GetMeasurements.Handle(ctx, query.GetMeasurements{
			AssetID: r.PathValue("asset_id"),
			From:    from,
			To:      to,
		})
		if err != nil {
			writeQueryError(w, r.WithContext(ctx), logger, err)
			return
		}

		writeJSON(w, http.StatusOK, series)
	})

	handler := http.Handler(mux)
	handler = withAccessLogging(handler, logger)
	handler = withRequestMetadata(handler)
	return handler
}

func parseTimeParam(r *http.Request, name string) (time.Time, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return time.Time{}, errors.New(name + " is required")
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New(name + " must be RFC3339")
	}

	return parsed.UTC(), nil
}

func writeQueryError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, query.ErrAssetIDRequired),
		errors.Is(err, query.ErrTimestampZero),
		errors.Is(err, query.ErrInvalidTimeRange),
		errors.Is(err, query.ErrDownstreamInvalidRequest):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, query.ErrMeasurementServiceUnavailable),
		errors.Is(err, context.DeadlineExceeded):
		if logger != nil {
			logger.WarnContext(r.Context(), "measurement service unavailable", "error", err)
		}
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "measurement service unavailable"})
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "get measurements failed", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}

func withAccessLogging(next http.Handler, logger *slog.Logger) http.Handler {
	if logger == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		route := r.Pattern
		if route == "" {
			route = r.URL.Path
		}

		cacheStatus := requestctx.CacheStatusFromContext(r.Context())
		if cacheStatus == "" {
			cacheStatus = requestctx.CacheStatusNotApplicable
		}

		logger.InfoContext(r.Context(), "handled http request",
			"method", r.Method,
			"route", route,
			"path", r.URL.Path,
			"status", recorder.StatusCode(),
			"duration", time.Since(start),
			"request_id", requestctx.RequestIDFromContext(r.Context()),
			"correlation_id", requestctx.CorrelationIDFromContext(r.Context()),
			"cache_status", cacheStatus,
		)
	})
}

func withRequestMetadata(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID, correlationID := requestctx.Normalize(
			r.Header.Get(requestctx.RequestIDHeader),
			r.Header.Get(requestctx.CorrelationIDHeader),
		)

		w.Header().Set(requestctx.RequestIDHeader, requestID)
		if correlationID != "" {
			w.Header().Set(requestctx.CorrelationIDHeader, correlationID)
		}

		ctx := requestctx.WithValues(r.Context(), requestID, correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}

	return r.ResponseWriter.Write(p)
}

func (r *statusRecorder) StatusCode() int {
	if r.statusCode == 0 {
		return http.StatusOK
	}

	return r.statusCode
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}
