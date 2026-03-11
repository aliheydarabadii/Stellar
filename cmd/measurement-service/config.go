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
	GRPCConnectionTimeout                   time.Duration `env:"GRPC_CONNECTION_TIMEOUT" envDefault:"5s"`
	GRPCMaxRecvMsgSizeBytes                 int           `env:"GRPC_MAX_RECV_MSG_SIZE_BYTES" envDefault:"4194304"`
	GRPCMaxSendMsgSizeBytes                 int           `env:"GRPC_MAX_SEND_MSG_SIZE_BYTES" envDefault:"4194304"`
	GRPCKeepaliveTime                       time.Duration `env:"GRPC_KEEPALIVE_TIME" envDefault:"2m"`
	GRPCKeepaliveTimeout                    time.Duration `env:"GRPC_KEEPALIVE_TIMEOUT" envDefault:"20s"`
	GRPCKeepaliveMinTime                    time.Duration `env:"GRPC_KEEPALIVE_MIN_TIME" envDefault:"1m"`
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
	case cfg.GRPCConnectionTimeout <= 0:
		return errors.New("GRPC_CONNECTION_TIMEOUT must be positive")
	case cfg.GRPCMaxRecvMsgSizeBytes <= 0:
		return errors.New("GRPC_MAX_RECV_MSG_SIZE_BYTES must be positive")
	case cfg.GRPCMaxSendMsgSizeBytes <= 0:
		return errors.New("GRPC_MAX_SEND_MSG_SIZE_BYTES must be positive")
	case cfg.GRPCKeepaliveTime <= 0:
		return errors.New("GRPC_KEEPALIVE_TIME must be positive")
	case cfg.GRPCKeepaliveTimeout <= 0:
		return errors.New("GRPC_KEEPALIVE_TIMEOUT must be positive")
	case cfg.GRPCKeepaliveMinTime <= 0:
		return errors.New("GRPC_KEEPALIVE_MIN_TIME must be positive")
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
	case "GRPCConnectionTimeout":
		return "GRPC_CONNECTION_TIMEOUT"
	case "GRPCMaxRecvMsgSizeBytes":
		return "GRPC_MAX_RECV_MSG_SIZE_BYTES"
	case "GRPCMaxSendMsgSizeBytes":
		return "GRPC_MAX_SEND_MSG_SIZE_BYTES"
	case "GRPCKeepaliveTime":
		return "GRPC_KEEPALIVE_TIME"
	case "GRPCKeepaliveTimeout":
		return "GRPC_KEEPALIVE_TIMEOUT"
	case "GRPCKeepaliveMinTime":
		return "GRPC_KEEPALIVE_MIN_TIME"
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
