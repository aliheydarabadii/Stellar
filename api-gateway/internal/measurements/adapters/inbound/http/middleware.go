package http

import (
	"log/slog"
	stdhttp "net/http"
	"time"

	"api_gateway/internal/platform/requestctx"
	"github.com/gin-gonic/gin"
)

func withAccessLogging(next stdhttp.Handler, logger *slog.Logger) stdhttp.Handler {
	if logger == nil {
		return next
	}

	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		route := requestctx.RouteFromContext(r.Context())
		if route == "" {
			route = r.Pattern
		}
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

func withRouteMetadata() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if route := c.FullPath(); route != "" {
			requestctx.SetRoute(c.Request.Context(), route)
		}
	}
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
