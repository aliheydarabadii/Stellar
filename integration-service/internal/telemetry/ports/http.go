package ports

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const shutdownTimeout = 5 * time.Second

type HTTPServer interface {
	Start(ctx context.Context) error
}

type Server struct {
	logger    *slog.Logger
	metrics   *Metrics
	readiness *Readiness
	server    *http.Server
}

func NewHTTPServer(addr string, logger *slog.Logger, metrics *Metrics, readiness *Readiness) (*Server, error) {
	if addr == "" {
		return nil, fmt.Errorf("http server address must not be empty")
	}

	if logger == nil {
		logger = slog.Default()
	}

	if metrics == nil {
		metrics = NewMetrics()
	}

	server := &Server{
		logger:    logger,
		metrics:   metrics,
		readiness: readiness,
		server:    &http.Server{Addr: addr},
	}
	server.server.Handler = server.newMux()

	return server, nil
}

func (s *Server) Start(ctx context.Context) error {
	shutdownErrCh := make(chan error, 1)

	go func() {
		<-ctx.Done()
		s.readiness.MarkShuttingDown()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		shutdownErrCh <- s.server.Shutdown(shutdownCtx)
	}()

	s.logger.Info("http server started", "addr", s.server.Addr)

	err := s.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	select {
	case shutdownErr := <-shutdownErrCh:
		if errors.Is(shutdownErr, context.Canceled) {
			return nil
		}
		return shutdownErr
	default:
		return nil
	}
}

func (s *Server) readyz(w http.ResponseWriter, _ *http.Request) {
	if !s.readiness.Ready(time.Now().UTC()) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/readyz", s.readyz)
	mux.Handle("/metrics", s.metrics)

	return mux
}

var _ HTTPServer = (*Server)(nil)
