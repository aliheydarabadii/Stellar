package main

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"stellar/internal/telemetry/adapters/influxdb"
	"stellar/internal/telemetry/adapters/modbus"
	"stellar/internal/telemetry/domain"
	"stellar/internal/telemetry/ports"
)

const (
	defaultInfluxTimeout  = 5 * time.Second
	minReadinessStaleness = 5 * time.Second
	readinessMultiplier   = 3
)

type config struct {
	AssetID             domain.AssetID
	AssetType           domain.AssetType
	PollInterval        time.Duration
	ReadinessStaleAfter time.Duration
	HTTPPort            int
	Modbus              modbus.Config
	Influx              influxdb.Config
	Tracing             ports.TracingConfig
}

type envConfig struct {
	AssetID                  string        `env:"ASSET_ID,required,notEmpty"`
	AssetType                string        `env:"ASSET_TYPE,required,notEmpty"`
	ModbusHost               string        `env:"MODBUS_HOST,required,notEmpty"`
	ModbusPort               uint16        `env:"MODBUS_PORT,required"`
	ModbusUnitID             uint8         `env:"MODBUS_UNIT_ID,required"`
	ModbusRegisterType       string        `env:"MODBUS_REGISTER_TYPE,required,notEmpty"`
	ModbusSetpointAddress    uint16        `env:"MODBUS_SETPOINT_ADDRESS,required"`
	ModbusActivePowerAddress uint16        `env:"MODBUS_ACTIVE_POWER_ADDRESS,required"`
	ModbusSignedValues       bool          `env:"MODBUS_SIGNED_VALUES,required"`
	PollInterval             time.Duration `env:"POLL_INTERVAL,required"`
	HTTPPort                 int           `env:"HTTP_PORT,required"`
	InfluxURL                string        `env:"INFLUX_URL,required,notEmpty"`
	InfluxToken              string        `env:"INFLUX_TOKEN,required,notEmpty"`
	InfluxOrg                string        `env:"INFLUX_ORG,required,notEmpty"`
	InfluxBucket             string        `env:"INFLUX_BUCKET,required,notEmpty"`
	InfluxWriteMode          string        `env:"INFLUX_WRITE_MODE" envDefault:"blocking"`
	InfluxBatchSize          uint          `env:"INFLUX_BATCH_SIZE"`
	InfluxLogLevel           uint          `env:"INFLUX_LOG_LEVEL"`
	InfluxFlushInterval      time.Duration `env:"INFLUX_FLUSH_INTERVAL"`
	TracingEnabled           bool          `env:"TRACING_ENABLED" envDefault:"false"`
	TracingEndpoint          string        `env:"TRACING_ENDPOINT"`
	TracingInsecure          bool          `env:"TRACING_INSECURE" envDefault:"true"`
	TracingSampleRatio       float64       `env:"TRACING_SAMPLE_RATIO" envDefault:"1.0"`
}

func loadConfig() (config, error) {
	var raw envConfig
	if err := env.Parse(&raw); err != nil {
		return config{}, fmt.Errorf("parse environment: %w", err)
	}

	if raw.PollInterval <= 0 {
		return config{}, fmt.Errorf("POLL_INTERVAL must be greater than zero")
	}

	if raw.HTTPPort <= 0 || raw.HTTPPort > 65535 {
		return config{}, fmt.Errorf("HTTP_PORT must be between 1 and 65535")
	}

	if raw.InfluxFlushInterval < 0 {
		return config{}, fmt.Errorf("INFLUX_FLUSH_INTERVAL must not be negative")
	}

	influxWriteMode, err := parseInfluxWriteMode(raw.InfluxWriteMode)
	if err != nil {
		return config{}, err
	}

	registerMapping, err := domain.NewRegisterMapping(
		domain.RegisterType(raw.ModbusRegisterType),
		raw.ModbusSetpointAddress,
		raw.ModbusActivePowerAddress,
		raw.ModbusSignedValues,
	)
	if err != nil {
		return config{}, fmt.Errorf("build register mapping: %w", err)
	}

	return config{
		AssetID:             domain.AssetID(raw.AssetID),
		AssetType:           domain.AssetType(raw.AssetType),
		PollInterval:        raw.PollInterval,
		ReadinessStaleAfter: readinessStaleness(raw.PollInterval),
		HTTPPort:            raw.HTTPPort,
		Modbus: modbus.Config{
			Host:            raw.ModbusHost,
			Port:            raw.ModbusPort,
			UnitID:          raw.ModbusUnitID,
			RegisterMapping: registerMapping,
		},
		Influx: influxdb.Config{
			BaseURL:       raw.InfluxURL,
			Org:           raw.InfluxOrg,
			Bucket:        raw.InfluxBucket,
			Token:         raw.InfluxToken,
			Timeout:       defaultInfluxTimeout,
			LogLevel:      raw.InfluxLogLevel,
			WriteMode:     influxWriteMode,
			BatchSize:     raw.InfluxBatchSize,
			FlushInterval: raw.InfluxFlushInterval,
		},
		Tracing: ports.TracingConfig{
			Enabled:     raw.TracingEnabled,
			Endpoint:    raw.TracingEndpoint,
			Insecure:    raw.TracingInsecure,
			SampleRatio: raw.TracingSampleRatio,
		},
	}, nil
}

func parseInfluxWriteMode(value string) (influxdb.WriteMode, error) {
	mode := influxdb.WriteMode(value)
	switch mode {
	case influxdb.WriteModeBlocking, influxdb.WriteModeBatch:
		return mode, nil
	default:
		return "", fmt.Errorf("INFLUX_WRITE_MODE must be one of %q or %q", influxdb.WriteModeBlocking, influxdb.WriteModeBatch)
	}
}

func readinessStaleness(pollInterval time.Duration) time.Duration {
	staleness := time.Duration(readinessMultiplier) * pollInterval
	if staleness < minReadinessStaleness {
		return minReadinessStaleness
	}

	return staleness
}
