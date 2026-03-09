package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"stellar/internal/telemetry/adapters/influxdb"
	"stellar/internal/telemetry/adapters/modbus"
	"stellar/internal/telemetry/app"
	"stellar/internal/telemetry/app/command"
	"stellar/internal/telemetry/ports"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addressMapper := modbus.NewAddressMapper()
	decoder := modbus.NewDecoder()
	source := modbus.NewSource(addressMapper, decoder)

	pointMapper := influxdb.NewPointMapper()
	repository := influxdb.NewMeasurementRepository(pointMapper)

	application := app.NewApplication(source, repository)

	healthServer := newHealthServer(":8080")
	worker := newWorkerLoop(application, 5*time.Second)

	if err := run(ctx, healthServer, worker); err != nil {
		log.Fatalf("integration service stopped with error: %v", err)
	}
}

func run(ctx context.Context, healthServer ports.HTTPServer, worker ports.Worker) error {
	errCh := make(chan error, 2)

	go func() {
		errCh <- healthServer.Start(ctx)
	}()

	go func() {
		errCh <- worker.Start(ctx)
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		if errors.Is(err, context.Canceled) || err == nil {
			return nil
		}
		return err
	}
}

type workerLoop struct {
	application app.Application
	interval    time.Duration
}

func newWorkerLoop(application app.Application, interval time.Duration) *workerLoop {
	return &workerLoop{
		application: application,
		interval:    interval,
	}
}

func (w *workerLoop) Start(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	if err := w.collectOnce(ctx); err != nil {
		log.Printf("initial telemetry collection skipped: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case tickTime := <-ticker.C:
			if err := w.application.Commands.CollectTelemetry.Handle(ctx, command.CollectTelemetry{
				CollectedAt: tickTime.UTC(),
			}); err != nil {
				log.Printf("telemetry collection failed: %v", err)
			}
		}
	}
}

func (w *workerLoop) collectOnce(ctx context.Context) error {
	return w.application.Commands.CollectTelemetry.Handle(ctx, command.CollectTelemetry{
		CollectedAt: time.Now().UTC(),
	})
}

type healthServer struct {
	server *http.Server
}

func newHealthServer(addr string) *healthServer {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &healthServer{
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
}

func (s *healthServer) Start(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		errCh <- s.server.Shutdown(shutdownCtx)
	}()

	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return <-errCh
}

var (
	_ ports.Worker     = (*workerLoop)(nil)
	_ ports.HTTPServer = (*healthServer)(nil)
)
