package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"google.golang.org/grpc"

	measurementsv1 "stellar/api/proto"
	"stellar/internal/measurements/adapters/influxdb"
	"stellar/internal/measurements/app"
	"stellar/internal/measurements/ports"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(logger); err != nil {
		logger.Error("measurement service stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	influxClient := influxdb2.NewClient(cfg.InfluxURL, cfg.InfluxToken)
	defer influxClient.Close()

	application, err := app.NewWithConfig(
		influxdb.NewReadModel(influxClient, cfg.InfluxOrg, cfg.InfluxBucket, cfg.QueryTimeout, influxdb.CircuitBreakerConfig{
			FailureThreshold:    cfg.InfluxCircuitBreakerFailureThreshold,
			OpenTimeout:         cfg.InfluxCircuitBreakerOpenTimeout,
			HalfOpenMaxRequests: cfg.InfluxCircuitBreakerHalfOpenMaxRequests,
		}),
		app.Config{
			MaxQueryRange: cfg.MaxQueryRange,
		},
	)
	if err != nil {
		logger.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer()
	measurementsv1.RegisterMeasurementServiceServer(grpcServer, ports.NewGRPCServer(application))

	grpcListener, err := net.Listen("tcp", cfg.GRPCListenAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC: %w", err)
	}

	var ready atomic.Bool
	healthServer := &http.Server{
		Addr:    cfg.HealthListenAddr,
		Handler: ports.NewHealthHandler(ready.Load),
	}

	serverErrors := make(chan error, 2)

	go func() {
		logger.Info("starting gRPC server", "address", cfg.GRPCListenAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			serverErrors <- fmt.Errorf("serve gRPC: %w", err)
		}
	}()

	go func() {
		logger.Info("starting health server", "address", cfg.HealthListenAddr)
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- fmt.Errorf("serve health: %w", err)
		}
	}()

	ready.Store(true)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrors:
		ready.Store(false)
		_ = shutdown(context.Background(), logger, grpcServer, healthServer)
		return err
	case <-ctx.Done():
		ready.Store(false)
		logger.Info("shutdown signal received")
		return shutdown(context.Background(), logger, grpcServer, healthServer)
	}
}

func shutdown(parent context.Context, logger *slog.Logger, grpcServer *grpc.Server, healthServer *http.Server) error {
	shutdownCtx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	grpcStopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcStopped)
	}()

	select {
	case <-grpcStopped:
	case <-shutdownCtx.Done():
		logger.Warn("gRPC graceful shutdown timed out, forcing stop")
		grpcServer.Stop()
	}

	if err := healthServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown health server: %w", err)
	}

	return nil
}
