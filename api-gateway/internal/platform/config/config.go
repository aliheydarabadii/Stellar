package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	env "github.com/caarlos0/env/v11"
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
	LogLevel                   string        `env:"LOG_LEVEL" envDefault:"INFO"`
	MeasurementServiceGRPCAddr string        `env:"MEASUREMENT_SERVICE_GRPC_ADDR"`
	HTTPListenAddr             string        `env:"HTTP_LISTEN_ADDR" envDefault:":8080"`
	HealthListenAddr           string        `env:"HEALTH_LISTEN_ADDR" envDefault:":8081"`
	RedisAddr                  string        `env:"REDIS_ADDR"`
	RedisUsername              string        `env:"REDIS_USERNAME"`
	RedisPassword              string        `env:"REDIS_PASSWORD"`
	RedisDB                    int           `env:"REDIS_DB"`
	CacheTTL                   time.Duration `env:"CACHE_TTL" envDefault:"5m"`
	RequestTimeout             time.Duration `env:"REQUEST_TIMEOUT" envDefault:"10s"`
	ReadinessCheckTimeout      time.Duration `env:"READINESS_CHECK_TIMEOUT" envDefault:"2s"`
	HTTPReadHeaderTimeout      time.Duration `env:"HTTP_READ_HEADER_TIMEOUT" envDefault:"5s"`
	HTTPReadTimeout            time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"10s"`
	HTTPWriteTimeout           time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"15s"`
	HTTPIdleTimeout            time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"60s"`
	HTTPMaxHeaderBytes         int           `env:"HTTP_MAX_HEADER_BYTES" envDefault:"1048576"`
}

func Load() (Config, error) {
	cfg, err := env.ParseAsWithOptions[Config](env.Options{
		Environment: loadEnvironment(),
	})
	if err != nil {
		return Config{}, translateConfigParseError(err)
	}

	if value, ok := os.LookupEnv("HTTP_LISTEN_ADDR"); ok && value == "" {
		cfg.HTTPListenAddr = ""
	}
	if value, ok := os.LookupEnv("HEALTH_LISTEN_ADDR"); ok && value == "" {
		cfg.HealthListenAddr = ""
	}

	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validateConfig(cfg Config) error {
	switch {
	case cfg.MeasurementServiceGRPCAddr == "":
		return errors.New("MEASUREMENT_SERVICE_GRPC_ADDR is required")
	case cfg.RedisAddr == "":
		return errors.New("REDIS_ADDR is required")
	case cfg.HTTPListenAddr == "":
		return errors.New("HTTP_LISTEN_ADDR must not be empty")
	case cfg.RedisDB < 0:
		return errors.New("REDIS_DB must not be negative")
	case cfg.CacheTTL <= 0:
		return errors.New("CACHE_TTL must be positive")
	case cfg.RequestTimeout < 0:
		return errors.New("REQUEST_TIMEOUT must not be negative")
	case cfg.ReadinessCheckTimeout <= 0:
		return errors.New("READINESS_CHECK_TIMEOUT must be positive")
	case cfg.HTTPReadHeaderTimeout <= 0:
		return errors.New("HTTP_READ_HEADER_TIMEOUT must be positive")
	case cfg.HTTPReadTimeout <= 0:
		return errors.New("HTTP_READ_TIMEOUT must be positive")
	case cfg.HTTPWriteTimeout <= 0:
		return errors.New("HTTP_WRITE_TIMEOUT must be positive")
	case cfg.HTTPIdleTimeout <= 0:
		return errors.New("HTTP_IDLE_TIMEOUT must be positive")
	case cfg.HTTPMaxHeaderBytes <= 0:
		return errors.New("HTTP_MAX_HEADER_BYTES must be positive")
	}

	return nil
}

func loadEnvironment() map[string]string {
	environment := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}

		if value == "" && key != "HTTP_LISTEN_ADDR" && key != "HEALTH_LISTEN_ADDR" {
			continue
		}

		environment[key] = value
	}

	return environment
}

func translateConfigParseError(err error) error {
	var parseErr env.ParseError
	if errors.As(err, &parseErr) {
		return fmt.Errorf("parse %s: %w", configFieldEnvName(parseErr.Name), parseErr.Err)
	}

	return err
}

func configFieldEnvName(fieldName string) string {
	switch fieldName {
	case "MeasurementServiceGRPCAddr":
		return "MEASUREMENT_SERVICE_GRPC_ADDR"
	case "LogLevel":
		return "LOG_LEVEL"
	case "HTTPListenAddr":
		return "HTTP_LISTEN_ADDR"
	case "HealthListenAddr":
		return "HEALTH_LISTEN_ADDR"
	case "RedisAddr":
		return "REDIS_ADDR"
	case "RedisUsername":
		return "REDIS_USERNAME"
	case "RedisPassword":
		return "REDIS_PASSWORD"
	case "RedisDB":
		return "REDIS_DB"
	case "CacheTTL":
		return "CACHE_TTL"
	case "RequestTimeout":
		return "REQUEST_TIMEOUT"
	case "ReadinessCheckTimeout":
		return "READINESS_CHECK_TIMEOUT"
	case "HTTPReadHeaderTimeout":
		return "HTTP_READ_HEADER_TIMEOUT"
	case "HTTPReadTimeout":
		return "HTTP_READ_TIMEOUT"
	case "HTTPWriteTimeout":
		return "HTTP_WRITE_TIMEOUT"
	case "HTTPIdleTimeout":
		return "HTTP_IDLE_TIMEOUT"
	case "HTTPMaxHeaderBytes":
		return "HTTP_MAX_HEADER_BYTES"
	default:
		return fieldName
	}
}
