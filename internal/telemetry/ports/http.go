package ports

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

const shutdownTimeout = 5 * time.Second

type HTTPServer interface {
	Start(ctx context.Context) error
}

type Server struct {
	logger *slog.Logger
	ready  atomic.Bool
	server *http.Server
}

func NewHTTPServer(addr string, logger *slog.Logger) (*Server, error) {
	if addr == "" {
		return nil, fmt.Errorf("http server address must not be empty")
	}

	if logger == nil {
		logger = slog.Default()
	}

	server := &Server{
		logger: logger,
		server: &http.Server{Addr: addr},
	}
	server.server.Handler = server.newMux()
	server.ready.Store(true)

	return server, nil
}

func (s *Server) Start(ctx context.Context) error {
	shutdownErrCh := make(chan error, 1)

	go func() {
		<-ctx.Done()
		s.ready.Store(false)

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
	if !s.ready.Load() {
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

	return mux
}

var _ HTTPServer = (*Server)(nil)
