package ports

import (
	"fmt"
	"net/http"
)

func NewHealthHandler(isReady func() bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeHealthResponse(w, http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if isReady != nil && !isReady() {
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
