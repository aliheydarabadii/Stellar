package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	stdhttp "net/http"
	"net/url"
	"time"

	"api_gateway/internal/measurements"
	"api_gateway/internal/measurements/application"
	"api_gateway/internal/platform/requestctx"

	"github.com/gin-gonic/gin"
)

const defaultQueryWindow = 5 * time.Minute

var defaultTimeSource = func() time.Time {
	return time.Now().UTC()
}

type errorResponse struct {
	Error string `json:"error"`
}

type resolvedTimeRange struct {
	from       time.Time
	to         time.Time
	latestOnly bool
}

func NewHandler(queryHandler application.QueryHandler, logger *slog.Logger, requestTimeout time.Duration) stdhttp.Handler {
	engine := gin.New()
	engine.Use(withRouteMetadata())
	engine.GET("/assets/:asset_id/measurements", func(c *gin.Context) {
		ctx := c.Request.Context()
		if requestTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, requestTimeout)
			defer cancel()
		}

		timeRange, err := resolveTimeRange(c.Query("from"), c.Query("to"))
		if err != nil {
			writeJSON(c.Writer, stdhttp.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		assetID := decodePathParam(c.Param("asset_id"))
		if timeRange.latestOnly {
			requestctx.SetLatestMeasurementsRead(ctx)
		}

		series, err := queryHandler.Handle(ctx, application.Query{
			AssetID: assetID,
			From:    timeRange.from,
			To:      timeRange.to,
		})
		if err != nil {
			writeQueryError(c.Writer, c.Request.WithContext(ctx), logger, err)
			return
		}

		if timeRange.latestOnly {
			series = latestMeasurementOnly(series)
		}

		if requestctx.CacheStatusFromContext(ctx) == "" {
			requestctx.SetCacheStatus(ctx, requestctx.CacheStatusNotApplicable)
		}

		writeJSON(c.Writer, stdhttp.StatusOK, series)
	})

	handler := stdhttp.Handler(engine)
	handler = withAccessLogging(handler, logger)
	handler = withRequestMetadata(handler)
	return handler
}

func resolveTimeRange(fromValue, toValue string) (resolvedTimeRange, error) {
	switch {
	case fromValue == "" && toValue == "":
		to := defaultWindowEnd(defaultTimeSource())
		return resolvedTimeRange{
			from:       to.Add(-defaultQueryWindow),
			to:         to,
			latestOnly: true,
		}, nil
	case fromValue == "":
		to, err := parseTimeParam(toValue, "to")
		if err != nil {
			return resolvedTimeRange{}, err
		}
		return resolvedTimeRange{
			from: to.Add(-defaultQueryWindow),
			to:   to,
		}, nil
	case toValue == "":
		from, err := parseTimeParam(fromValue, "from")
		if err != nil {
			return resolvedTimeRange{}, err
		}
		return resolvedTimeRange{
			from: from,
			to:   from.Add(defaultQueryWindow),
		}, nil
	default:
		from, err := parseTimeParam(fromValue, "from")
		if err != nil {
			return resolvedTimeRange{}, err
		}

		to, err := parseTimeParam(toValue, "to")
		if err != nil {
			return resolvedTimeRange{}, err
		}

		return resolvedTimeRange{
			from: from,
			to:   to,
		}, nil
	}
}

func defaultWindowEnd(now time.Time) time.Time {
	return now.UTC().Truncate(defaultQueryWindow)
}

func latestMeasurementOnly(series measurements.MeasurementSeries) measurements.MeasurementSeries {
	if len(series.Points) <= 1 {
		return series
	}

	return measurements.MeasurementSeries{
		AssetID: series.AssetID,
		Points:  []measurements.MeasurementPoint{series.Points[len(series.Points)-1]},
	}
}

func parseTimeParam(value, name string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New(name + " is required")
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New(name + " must be RFC3339")
	}

	return parsed.UTC(), nil
}

func decodePathParam(value string) string {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}

	return decoded
}

func writeQueryError(w stdhttp.ResponseWriter, r *stdhttp.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, application.ErrAssetIDRequired),
		errors.Is(err, application.ErrTimestampZero),
		errors.Is(err, application.ErrInvalidTimeRange),
		errors.Is(err, application.ErrDownstreamInvalidRequest):
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, application.ErrMeasurementServiceUnavailable),
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

func writeJSON(w stdhttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}
