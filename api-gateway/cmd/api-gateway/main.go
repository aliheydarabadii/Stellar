package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	inboundhttp "api_gateway/internal/measurements/adapters/inbound/http"
	grpcadapter "api_gateway/internal/measurements/adapters/outbound/grpc"
	redisadapter "api_gateway/internal/measurements/adapters/outbound/redis"
	getmeasurements "api_gateway/internal/measurements/application"
	"api_gateway/internal/platform/config"
	"api_gateway/internal/platform/logging"
)

type readinessDependency interface {
	Ready(context.Context) error
}

func main() {
	bootstrapLogger, _ := logging.NewLogger(logging.DefaultLogLevel)

	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("api gateway stopped", "error", err)
		os.Exit(1)
	}

	logger, err := logging.NewLogger(cfg.LogLevel)
	if err != nil {
		bootstrapLogger.Error("api gateway stopped", "error", err)
		os.Exit(1)
	}

	if err := run(cfg, logger); err != nil {
		logger.Error("api gateway stopped", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	measurementsClient, err := grpcadapter.Dial(startupCtx, cfg.MeasurementServiceGRPCAddr)
	if err != nil {
		return fmt.Errorf("initialize measurement service client: %w", err)
	}
	defer func() {
		if err := measurementsClient.Close(); err != nil {
			logger.Warn("close measurement service client", "error", err)
		}
	}()

	measurementsCache, err := redisadapter.NewCache(startupCtx, cfg.RedisAddr, cfg.RedisUsername, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		return fmt.Errorf("initialize redis cache: %w", err)
	}
	defer func() {
		if err := measurementsCache.Close(); err != nil {
			logger.Warn("close redis cache", "error", err)
		}
	}()

	protectedMeasurementsReader, err := grpcadapter.NewCircuitBreakerReader(measurementsClient, logger)
	if err != nil {
		return fmt.Errorf("initialize measurements circuit breaker: %w", err)
	}

	measurementsReader, err := redisadapter.NewCachedReader(
		protectedMeasurementsReader,
		measurementsCache,
		cfg.CacheTTL,
		redisadapter.MeasurementsKey,
		logging.NewCacheObserver(logger),
	)
	if err != nil {
		return fmt.Errorf("initialize cached measurements reader: %w", err)
	}

	useCase, err := getmeasurements.NewMeasurementsHandler(measurementsReader)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}

	var ready atomic.Bool

	apiHandler := inboundhttp.NewHandler(useCase, logger, cfg.RequestTimeout)
	readinessCheck := newReadinessCheck(&ready, measurementsCache, measurementsClient)
	healthHandler := inboundhttp.NewHealthHandler(readinessCheck, cfg.ReadinessCheckTimeout)

	httpHandler := apiHandler
	var healthServer *http.Server
	if cfg.HealthListenAddr == "" || cfg.HealthListenAddr == cfg.HTTPListenAddr {
		rootMux := http.NewServeMux()
		rootMux.Handle("/healthz", healthHandler)
		rootMux.Handle("/readyz", healthHandler)
		rootMux.Handle("/", apiHandler)
		httpHandler = rootMux
	} else {
		healthServer = newHTTPServer(cfg.HealthListenAddr, healthHandler, cfg)
	}

	httpServer := newHTTPServer(cfg.HTTPListenAddr, httpHandler, cfg)

	serverErrors := make(chan error, 2)

	go func() {
		logger.Info("starting http server", "address", cfg.HTTPListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- fmt.Errorf("serve http: %w", err)
		}
	}()

	if healthServer != nil {
		go func() {
			logger.Info("starting health server", "address", cfg.HealthListenAddr)
			if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErrors <- fmt.Errorf("serve health: %w", err)
			}
		}()
	}

	ready.Store(true)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrors:
		ready.Store(false)
		_ = shutdown(context.Background(), logger, httpServer, healthServer)
		return err
	case <-ctx.Done():
		ready.Store(false)
		logger.Info("shutdown signal received")
		return shutdown(context.Background(), logger, httpServer, healthServer)
	}
}

func shutdown(parent context.Context, logger *slog.Logger, httpServer, healthServer *http.Server) error {
	shutdownCtx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	if healthServer != nil {
		if err := healthServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("shutdown health server: %w", err)
		}
	}

	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Warn("http graceful shutdown failed", "error", err)
		return fmt.Errorf("shutdown http server: %w", err)
	}

	return nil
}

func newReadinessCheck(started *atomic.Bool, deps ...readinessDependency) inboundhttp.ReadinessProbe {
	return func(ctx context.Context) error {
		if started != nil && !started.Load() {
			return errors.New("service is not ready")
		}

		for _, dep := range deps {
			if dep == nil {
				continue
			}
			if err := dep.Ready(ctx); err != nil {
				return err
			}
		}

		return nil
	}
}

func newHTTPServer(addr string, handler http.Handler, cfg config.Config) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		MaxHeaderBytes:    cfg.HTTPMaxHeaderBytes,
	}
}
