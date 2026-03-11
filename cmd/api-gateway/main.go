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
	healthHandler := ports.NewHealthHandler(ready.Load)

	httpHandler := apiHandler
	var healthServer *http.Server
	if cfg.HealthListenAddr == "" || cfg.HealthListenAddr == cfg.HTTPListenAddr {
		rootMux := http.NewServeMux()
		rootMux.Handle("/healthz", healthHandler)
		rootMux.Handle("/readyz", healthHandler)
		rootMux.Handle("/", apiHandler)
		httpHandler = rootMux
	} else {
		healthServer = &http.Server{
			Addr:              cfg.HealthListenAddr,
			Handler:           healthHandler,
			ReadHeaderTimeout: 5 * time.Second,
		}
	}

	httpServer := &http.Server{
		Addr:              cfg.HTTPListenAddr,
		Handler:           httpHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}

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
		CacheTTL:                   5 * time.Minute,
		RequestTimeout:             10 * time.Second,
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
