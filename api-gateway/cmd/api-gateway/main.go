package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	cacheadapter "api_gateway/internal/gateway/adapters/cache"
	grpcadapter "api_gateway/internal/gateway/adapters/grpc"
	"api_gateway/internal/gateway/app"
	"api_gateway/internal/gateway/ports"
)

const (
	defaultCacheTTL              = 5 * time.Minute
	defaultRequestTimeout        = 10 * time.Second
	defaultReadinessCheckTimeout = 2 * time.Second
	defaultHTTPReadHeaderTimeout = 5 * time.Second
	defaultHTTPReadTimeout       = 10 * time.Second
	defaultHTTPWriteTimeout      = 15 * time.Second
	defaultHTTPIdleTimeout       = 60 * time.Second
	defaultHTTPMaxHeaderBytes    = 1 << 20
)

type config struct {
	MeasurementServiceGRPCAddr string
	HTTPListenAddr             string
	HealthListenAddr           string
	RedisAddr                  string
	RedisUsername              string
	RedisPassword              string
	RedisDB                    int
	CacheTTL                   time.Duration
	RequestTimeout             time.Duration
	ReadinessCheckTimeout      time.Duration
	HTTPReadHeaderTimeout      time.Duration
	HTTPReadTimeout            time.Duration
	HTTPWriteTimeout           time.Duration
	HTTPIdleTimeout            time.Duration
	HTTPMaxHeaderBytes         int
}

type readinessDependency interface {
	Ready(context.Context) error
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(logger); err != nil {
		logger.Error("api gateway stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

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

	measurementsCache, err := cacheadapter.NewRedisCache(startupCtx, cfg.RedisAddr, cfg.RedisUsername, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		return fmt.Errorf("initialize redis cache: %w", err)
	}
	defer func() {
		if err := measurementsCache.Close(); err != nil {
			logger.Warn("close redis cache", "error", err)
		}
	}()

	application, err := app.New(measurementsClient, measurementsCache, cfg.CacheTTL, cacheadapter.MeasurementsKey, logger)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}

	var ready atomic.Bool

	apiHandler := ports.NewHTTPHandler(application, logger, cfg.RequestTimeout)
	readinessCheck := newReadinessCheck(&ready, measurementsCache, measurementsClient)
	healthHandler := ports.NewHealthHandler(readinessCheck, cfg.ReadinessCheckTimeout)

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

func loadConfig() (config, error) {
	cfg := config{
		MeasurementServiceGRPCAddr: os.Getenv("MEASUREMENT_SERVICE_GRPC_ADDR"),
		HTTPListenAddr:             envOrDefault("HTTP_LISTEN_ADDR", ":8080"),
		HealthListenAddr:           envOrDefault("HEALTH_LISTEN_ADDR", ":8081"),
		RedisAddr:                  os.Getenv("REDIS_ADDR"),
		RedisUsername:              os.Getenv("REDIS_USERNAME"),
		RedisPassword:              os.Getenv("REDIS_PASSWORD"),
		CacheTTL:                   defaultCacheTTL,
		RequestTimeout:             defaultRequestTimeout,
		ReadinessCheckTimeout:      defaultReadinessCheckTimeout,
		HTTPReadHeaderTimeout:      defaultHTTPReadHeaderTimeout,
		HTTPReadTimeout:            defaultHTTPReadTimeout,
		HTTPWriteTimeout:           defaultHTTPWriteTimeout,
		HTTPIdleTimeout:            defaultHTTPIdleTimeout,
		HTTPMaxHeaderBytes:         defaultHTTPMaxHeaderBytes,
	}

	if value := os.Getenv("REDIS_DB"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return config{}, fmt.Errorf("parse REDIS_DB: %w", err)
		}
		if parsed < 0 {
			return config{}, errors.New("REDIS_DB must not be negative")
		}
		cfg.RedisDB = parsed
	}

	if value := os.Getenv("CACHE_TTL"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return config{}, fmt.Errorf("parse CACHE_TTL: %w", err)
		}
		if parsed <= 0 {
			return config{}, errors.New("CACHE_TTL must be positive")
		}
		cfg.CacheTTL = parsed
	}

	if value := os.Getenv("REQUEST_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return config{}, fmt.Errorf("parse REQUEST_TIMEOUT: %w", err)
		}
		if parsed < 0 {
			return config{}, errors.New("REQUEST_TIMEOUT must not be negative")
		}
		cfg.RequestTimeout = parsed
	}

	if value := os.Getenv("READINESS_CHECK_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return config{}, fmt.Errorf("parse READINESS_CHECK_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return config{}, errors.New("READINESS_CHECK_TIMEOUT must be positive")
		}
		cfg.ReadinessCheckTimeout = parsed
	}

	if value := os.Getenv("HTTP_READ_HEADER_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return config{}, fmt.Errorf("parse HTTP_READ_HEADER_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return config{}, errors.New("HTTP_READ_HEADER_TIMEOUT must be positive")
		}
		cfg.HTTPReadHeaderTimeout = parsed
	}

	if value := os.Getenv("HTTP_READ_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return config{}, fmt.Errorf("parse HTTP_READ_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return config{}, errors.New("HTTP_READ_TIMEOUT must be positive")
		}
		cfg.HTTPReadTimeout = parsed
	}

	if value := os.Getenv("HTTP_WRITE_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return config{}, fmt.Errorf("parse HTTP_WRITE_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return config{}, errors.New("HTTP_WRITE_TIMEOUT must be positive")
		}
		cfg.HTTPWriteTimeout = parsed
	}

	if value := os.Getenv("HTTP_IDLE_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return config{}, fmt.Errorf("parse HTTP_IDLE_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return config{}, errors.New("HTTP_IDLE_TIMEOUT must be positive")
		}
		cfg.HTTPIdleTimeout = parsed
	}

	if value := os.Getenv("HTTP_MAX_HEADER_BYTES"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return config{}, fmt.Errorf("parse HTTP_MAX_HEADER_BYTES: %w", err)
		}
		if parsed <= 0 {
			return config{}, errors.New("HTTP_MAX_HEADER_BYTES must be positive")
		}
		cfg.HTTPMaxHeaderBytes = parsed
	}

	switch {
	case cfg.MeasurementServiceGRPCAddr == "":
		return config{}, errors.New("MEASUREMENT_SERVICE_GRPC_ADDR is required")
	case cfg.RedisAddr == "":
		return config{}, errors.New("REDIS_ADDR is required")
	case cfg.HTTPListenAddr == "":
		return config{}, errors.New("HTTP_LISTEN_ADDR must not be empty")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func newReadinessCheck(started *atomic.Bool, deps ...readinessDependency) ports.ReadinessProbe {
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

func newHTTPServer(addr string, handler http.Handler, cfg config) *http.Server {
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
