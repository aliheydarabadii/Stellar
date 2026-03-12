package http

import (
	"context"
	"fmt"
	stdhttp "net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type ReadinessProbe func(context.Context) error

func NewHealthHandler(readinessProbe ReadinessProbe, readinessTimeout time.Duration) stdhttp.Handler {
	engine := gin.New()
	engine.GET("/healthz", func(c *gin.Context) {
		writeHealthResponse(c.Writer, stdhttp.StatusOK)
	})
	engine.GET("/readyz", func(c *gin.Context) {
		if readinessProbe == nil {
			writeHealthResponse(c.Writer, stdhttp.StatusOK)
			return
		}

		ctx := c.Request.Context()
		if readinessTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, readinessTimeout)
			defer cancel()
		}

		if err := readinessProbe(ctx); err != nil {
			writeHealthResponse(c.Writer, stdhttp.StatusServiceUnavailable)
			return
		}

		writeHealthResponse(c.Writer, stdhttp.StatusOK)
	})

	return engine
}

func writeHealthResponse(w stdhttp.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintln(w, stdhttp.StatusText(status))
}
