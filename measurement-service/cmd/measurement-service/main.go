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

	grpcadapter "stellar/internal/measurements/adapters/inbound/grpc"
	"stellar/internal/measurements/adapters/outbound/influxdb"
	getmeasurements "stellar/internal/measurements/application/get_measurements"
	"stellar/internal/platform/config"
	"stellar/internal/platform/health"
	"stellar/internal/platform/logging"
)

func main() {
	bootstrapLogger := logging.NewDefaultLogger()

	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("measurement service stopped", "error", err)
		os.Exit(1)
	}

	logger, err := logging.NewLogger(cfg.LogLevel)
	if err != nil {
		bootstrapLogger.Error("measurement service stopped", "error", err)
		os.Exit(1)
	}

	if err := run(cfg, logger); err != nil {
		logger.Error("measurement service stopped", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	influxClient := influxdb2.NewClient(cfg.InfluxURL, cfg.InfluxToken)
	defer influxClient.Close()

	getMeasurements, err := getmeasurements.NewUseCaseWithConfig(
		influxdb.NewReadModel(influxClient, cfg.InfluxOrg, cfg.InfluxBucket, cfg.QueryTimeout, influxdb.CircuitBreakerConfig{
			FailureThreshold:    cfg.InfluxCircuitBreakerFailureThreshold,
			OpenTimeout:         cfg.InfluxCircuitBreakerOpenTimeout,
			HalfOpenMaxRequests: cfg.InfluxCircuitBreakerHalfOpenMaxRequests,
		}),
		getmeasurements.Config{
			MaxQueryRange: cfg.MaxQueryRange,
		},
	)
	if err != nil {
		return fmt.Errorf("initialize get measurements use case: %w", err)
	}
	grpcServer := grpcadapter.NewTransport(logger, getMeasurements, grpcadapter.TransportConfig{
		ConnectionTimeout:   cfg.GRPCConnectionTimeout,
		MaxRecvMsgSizeBytes: cfg.GRPCMaxRecvMsgSizeBytes,
		MaxSendMsgSizeBytes: cfg.GRPCMaxSendMsgSizeBytes,
		KeepaliveTime:       cfg.GRPCKeepaliveTime,
		KeepaliveTimeout:    cfg.GRPCKeepaliveTimeout,
		KeepaliveMinTime:    cfg.GRPCKeepaliveMinTime,
	})

	grpcListener, err := net.Listen("tcp", cfg.GRPCListenAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC: %w", err)
	}

	var ready atomic.Bool
	healthServer := &http.Server{
		Addr:    cfg.HealthListenAddr,
		Handler: health.NewHealthHandler(ready.Load),
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
