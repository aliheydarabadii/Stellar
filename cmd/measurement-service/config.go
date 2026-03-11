package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type config struct {
	InfluxURL                               string        `env:"INFLUX_URL"`
	InfluxOrg                               string        `env:"INFLUX_ORG"`
	InfluxBucket                            string        `env:"INFLUX_BUCKET"`
	InfluxToken                             string        `env:"INFLUX_TOKEN"`
	GRPCListenAddr                          string        `env:"GRPC_LISTEN_ADDR" envDefault:":9090"`
	HealthListenAddr                        string        `env:"HEALTH_LISTEN_ADDR" envDefault:":8080"`
	MaxQueryRange                           time.Duration `env:"MAX_QUERY_RANGE" envDefault:"15m"`
	QueryTimeout                            time.Duration `env:"QUERY_TIMEOUT" envDefault:"10s"`
	InfluxCircuitBreakerFailureThreshold    int           `env:"INFLUX_CIRCUIT_BREAKER_FAILURE_THRESHOLD" envDefault:"5"`
	InfluxCircuitBreakerOpenTimeout         time.Duration `env:"INFLUX_CIRCUIT_BREAKER_OPEN_TIMEOUT" envDefault:"30s"`
	InfluxCircuitBreakerHalfOpenMaxRequests int           `env:"INFLUX_CIRCUIT_BREAKER_HALF_OPEN_MAX_REQUESTS" envDefault:"1"`
}

func loadConfig() (config, error) {
	cfg, err := env.ParseAs[config]()
	if err != nil {
		return config{}, translateConfigParseError(err)
	}

	if err := validateConfig(cfg); err != nil {
		return config{}, err
	}

	return cfg, nil
}

func validateConfig(cfg config) error {
	switch {
	case cfg.InfluxURL == "":
		return errors.New("INFLUX_URL is required")
	case cfg.InfluxOrg == "":
		return errors.New("INFLUX_ORG is required")
	case cfg.InfluxBucket == "":
		return errors.New("INFLUX_BUCKET is required")
	case cfg.InfluxToken == "":
		return errors.New("INFLUX_TOKEN is required")
	case cfg.MaxQueryRange <= 0:
		return errors.New("MAX_QUERY_RANGE must be positive")
	case cfg.QueryTimeout <= 0:
		return errors.New("QUERY_TIMEOUT must be positive")
	case cfg.InfluxCircuitBreakerFailureThreshold <= 0:
		return errors.New("INFLUX_CIRCUIT_BREAKER_FAILURE_THRESHOLD must be positive")
	case cfg.InfluxCircuitBreakerOpenTimeout <= 0:
		return errors.New("INFLUX_CIRCUIT_BREAKER_OPEN_TIMEOUT must be positive")
	case cfg.InfluxCircuitBreakerHalfOpenMaxRequests <= 0:
		return errors.New("INFLUX_CIRCUIT_BREAKER_HALF_OPEN_MAX_REQUESTS must be positive")
	}

	return nil
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
	case "InfluxURL":
		return "INFLUX_URL"
	case "InfluxOrg":
		return "INFLUX_ORG"
	case "InfluxBucket":
		return "INFLUX_BUCKET"
	case "InfluxToken":
		return "INFLUX_TOKEN"
	case "GRPCListenAddr":
		return "GRPC_LISTEN_ADDR"
	case "HealthListenAddr":
		return "HEALTH_LISTEN_ADDR"
	case "MaxQueryRange":
		return "MAX_QUERY_RANGE"
	case "QueryTimeout":
		return "QUERY_TIMEOUT"
	case "InfluxCircuitBreakerFailureThreshold":
		return "INFLUX_CIRCUIT_BREAKER_FAILURE_THRESHOLD"
	case "InfluxCircuitBreakerOpenTimeout":
		return "INFLUX_CIRCUIT_BREAKER_OPEN_TIMEOUT"
	case "InfluxCircuitBreakerHalfOpenMaxRequests":
		return "INFLUX_CIRCUIT_BREAKER_HALF_OPEN_MAX_REQUESTS"
	default:
		return fieldName
	}
}
