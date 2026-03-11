package ports

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type ReadinessProbe func(context.Context) error

func NewHealthHandler(readinessProbe ReadinessProbe, readinessTimeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeHealthResponse(w, http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if readinessProbe == nil {
			writeHealthResponse(w, http.StatusOK)
			return
		}

		ctx := r.Context()
		if readinessTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, readinessTimeout)
			defer cancel()
		}

		if err := readinessProbe(ctx); err != nil {
			writeHealthResponse(w, http.StatusServiceUnavailable)
			return
		}

		writeHealthResponse(w, http.StatusOK)
	})

	return mux
}

func writeHealthResponse(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintln(w, http.StatusText(status))
}
