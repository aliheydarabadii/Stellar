package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	DefaultCacheTTL              = 5 * time.Minute
	DefaultRequestTimeout        = 10 * time.Second
	DefaultReadinessCheckTimeout = 2 * time.Second
	DefaultHTTPReadHeaderTimeout = 5 * time.Second
	DefaultHTTPReadTimeout       = 10 * time.Second
	DefaultHTTPWriteTimeout      = 15 * time.Second
	DefaultHTTPIdleTimeout       = 60 * time.Second
	DefaultHTTPMaxHeaderBytes    = 1 << 20
)

type Config struct {
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

func Load() (Config, error) {
	cfg := Config{
		MeasurementServiceGRPCAddr: os.Getenv("MEASUREMENT_SERVICE_GRPC_ADDR"),
		HTTPListenAddr:             envOrDefault("HTTP_LISTEN_ADDR", ":8080"),
		HealthListenAddr:           envOrDefault("HEALTH_LISTEN_ADDR", ":8081"),
		RedisAddr:                  os.Getenv("REDIS_ADDR"),
		RedisUsername:              os.Getenv("REDIS_USERNAME"),
		RedisPassword:              os.Getenv("REDIS_PASSWORD"),
		CacheTTL:                   DefaultCacheTTL,
		RequestTimeout:             DefaultRequestTimeout,
		ReadinessCheckTimeout:      DefaultReadinessCheckTimeout,
		HTTPReadHeaderTimeout:      DefaultHTTPReadHeaderTimeout,
		HTTPReadTimeout:            DefaultHTTPReadTimeout,
		HTTPWriteTimeout:           DefaultHTTPWriteTimeout,
		HTTPIdleTimeout:            DefaultHTTPIdleTimeout,
		HTTPMaxHeaderBytes:         DefaultHTTPMaxHeaderBytes,
	}

	if value := os.Getenv("REDIS_DB"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse REDIS_DB: %w", err)
		}
		if parsed < 0 {
			return Config{}, errors.New("REDIS_DB must not be negative")
		}
		cfg.RedisDB = parsed
	}

	if value := os.Getenv("CACHE_TTL"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse CACHE_TTL: %w", err)
		}
		if parsed <= 0 {
			return Config{}, errors.New("CACHE_TTL must be positive")
		}
		cfg.CacheTTL = parsed
	}

	if value := os.Getenv("REQUEST_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse REQUEST_TIMEOUT: %w", err)
		}
		if parsed < 0 {
			return Config{}, errors.New("REQUEST_TIMEOUT must not be negative")
		}
		cfg.RequestTimeout = parsed
	}

	if value := os.Getenv("READINESS_CHECK_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse READINESS_CHECK_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return Config{}, errors.New("READINESS_CHECK_TIMEOUT must be positive")
		}
		cfg.ReadinessCheckTimeout = parsed
	}

	if value := os.Getenv("HTTP_READ_HEADER_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse HTTP_READ_HEADER_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return Config{}, errors.New("HTTP_READ_HEADER_TIMEOUT must be positive")
		}
		cfg.HTTPReadHeaderTimeout = parsed
	}

	if value := os.Getenv("HTTP_READ_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse HTTP_READ_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return Config{}, errors.New("HTTP_READ_TIMEOUT must be positive")
		}
		cfg.HTTPReadTimeout = parsed
	}

	if value := os.Getenv("HTTP_WRITE_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse HTTP_WRITE_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return Config{}, errors.New("HTTP_WRITE_TIMEOUT must be positive")
		}
		cfg.HTTPWriteTimeout = parsed
	}

	if value := os.Getenv("HTTP_IDLE_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse HTTP_IDLE_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return Config{}, errors.New("HTTP_IDLE_TIMEOUT must be positive")
		}
		cfg.HTTPIdleTimeout = parsed
	}

	if value := os.Getenv("HTTP_MAX_HEADER_BYTES"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse HTTP_MAX_HEADER_BYTES: %w", err)
		}
		if parsed <= 0 {
			return Config{}, errors.New("HTTP_MAX_HEADER_BYTES must be positive")
		}
		cfg.HTTPMaxHeaderBytes = parsed
	}

	switch {
	case cfg.MeasurementServiceGRPCAddr == "":
		return Config{}, errors.New("MEASUREMENT_SERVICE_GRPC_ADDR is required")
	case cfg.RedisAddr == "":
		return Config{}, errors.New("REDIS_ADDR is required")
	case cfg.HTTPListenAddr == "":
		return Config{}, errors.New("HTTP_LISTEN_ADDR must not be empty")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
