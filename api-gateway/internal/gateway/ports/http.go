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

	return withRequestMetadata(mux)
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
		errors.Is(err, query.ErrInvalidTimeRange):
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}
