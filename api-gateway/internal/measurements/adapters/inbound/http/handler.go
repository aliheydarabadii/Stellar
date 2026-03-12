package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	stdhttp "net/http"
	"time"

	getmeasurements "api_gateway/internal/measurements/application/get_measurements"
	"api_gateway/internal/platform/requestctx"
)

type errorResponse struct {
	Error string `json:"error"`
}

func NewHandler(useCase getmeasurements.UseCase, logger *slog.Logger, requestTimeout time.Duration) stdhttp.Handler {
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("GET /assets/{asset_id}/measurements", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		ctx := r.Context()
		if requestTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, requestTimeout)
			defer cancel()
		}

		from, err := parseTimeParam(r, "from")
		if err != nil {
			writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		to, err := parseTimeParam(r, "to")
		if err != nil {
			writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		series, err := useCase.Handle(ctx, getmeasurements.Query{
			AssetID: r.PathValue("asset_id"),
			From:    from,
			To:      to,
		})
		if err != nil {
			writeQueryError(w, r.WithContext(ctx), logger, err)
			return
		}

		writeJSON(w, stdhttp.StatusOK, series)
	})

	handler := stdhttp.Handler(mux)
	handler = withAccessLogging(handler, logger)
	handler = withRequestMetadata(handler)
	return handler
}

func parseTimeParam(r *stdhttp.Request, name string) (time.Time, error) {
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

func writeQueryError(w stdhttp.ResponseWriter, r *stdhttp.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, getmeasurements.ErrAssetIDRequired),
		errors.Is(err, getmeasurements.ErrTimestampZero),
		errors.Is(err, getmeasurements.ErrInvalidTimeRange),
		errors.Is(err, getmeasurements.ErrDownstreamInvalidRequest):
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, getmeasurements.ErrMeasurementServiceUnavailable),
		errors.Is(err, context.DeadlineExceeded):
		if logger != nil {
			logger.WarnContext(r.Context(), "measurement service unavailable", "error", err)
		}
		writeJSON(w, stdhttp.StatusServiceUnavailable, errorResponse{Error: "measurement service unavailable"})
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "get measurements failed", "error", err)
		}
		writeJSON(w, stdhttp.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}

func withAccessLogging(next stdhttp.Handler, logger *slog.Logger) stdhttp.Handler {
	if logger == nil {
		return next
	}

	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
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

func withRequestMetadata(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
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
	stdhttp.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = stdhttp.StatusOK
	}

	return r.ResponseWriter.Write(p)
}

func (r *statusRecorder) StatusCode() int {
	if r.statusCode == 0 {
		return stdhttp.StatusOK
	}

	return r.statusCode
}

func writeJSON(w stdhttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}
