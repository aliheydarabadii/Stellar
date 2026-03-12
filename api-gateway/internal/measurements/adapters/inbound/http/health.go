package http

import (
	"context"
	"fmt"
	stdhttp "net/http"
	"time"
)

type ReadinessProbe func(context.Context) error

func NewHealthHandler(readinessProbe ReadinessProbe, readinessTimeout time.Duration) stdhttp.Handler {
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("/healthz", func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writeHealthResponse(w, stdhttp.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if readinessProbe == nil {
			writeHealthResponse(w, stdhttp.StatusOK)
			return
		}

		ctx := r.Context()
		if readinessTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, readinessTimeout)
			defer cancel()
		}

		if err := readinessProbe(ctx); err != nil {
			writeHealthResponse(w, stdhttp.StatusServiceUnavailable)
			return
		}

		writeHealthResponse(w, stdhttp.StatusOK)
	})

	return mux
}

func writeHealthResponse(w stdhttp.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintln(w, stdhttp.StatusText(status))
}
