package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	tracingplatform "stellar/internal/platform/tracing"
	influxdbadapter "stellar/internal/telemetry/adapters/outbound/influxdb"
	modbusadapter "stellar/internal/telemetry/adapters/outbound/modbus"
	"stellar/internal/telemetry/domain"
)

const (
	defaultInfluxTimeout  = 5 * time.Second
	minReadinessStaleness = 5 * time.Second
	readinessMultiplier   = 3
)

type Config struct {
	LogLevel            string
	AssetID             domain.AssetID
	AssetType           domain.AssetType
	PollInterval        time.Duration
	ReadinessStaleAfter time.Duration
	HTTPPort            int
	Modbus              modbusadapter.Config
	Influx              influxdbadapter.Config
	Tracing             tracingplatform.TracingConfig
}

type envConfig struct {
	LogLevel                 string        `env:"LOG_LEVEL" envDefault:"INFO"`
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

func Load() (Config, error) {
	raw, err := env.ParseAs[envConfig]()
	if err != nil {
		return Config{}, translateConfigParseError(err)
	}

	if raw.PollInterval <= 0 {
		return Config{}, errors.New("POLL_INTERVAL must be greater than zero")
	}

	if raw.HTTPPort <= 0 || raw.HTTPPort > 65535 {
		return Config{}, errors.New("HTTP_PORT must be between 1 and 65535")
	}

	if raw.InfluxFlushInterval < 0 {
		return Config{}, errors.New("INFLUX_FLUSH_INTERVAL must not be negative")
	}

	influxWriteMode, err := parseInfluxWriteMode(raw.InfluxWriteMode)
	if err != nil {
		return Config{}, err
	}

	registerMapping, err := domain.NewRegisterMapping(
		domain.RegisterType(raw.ModbusRegisterType),
		raw.ModbusSetpointAddress,
		raw.ModbusActivePowerAddress,
		raw.ModbusSignedValues,
	)
	if err != nil {
		return Config{}, fmt.Errorf("build register mapping: %w", err)
	}

	return Config{
		LogLevel:            raw.LogLevel,
		AssetID:             domain.AssetID(raw.AssetID),
		AssetType:           domain.AssetType(raw.AssetType),
		PollInterval:        raw.PollInterval,
		ReadinessStaleAfter: readinessStaleness(raw.PollInterval),
		HTTPPort:            raw.HTTPPort,
		Modbus: modbusadapter.Config{
			Host:            raw.ModbusHost,
			Port:            raw.ModbusPort,
			UnitID:          raw.ModbusUnitID,
			RegisterMapping: registerMapping,
		},
		Influx: influxdbadapter.Config{
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
		Tracing: tracingplatform.TracingConfig{
			Enabled:     raw.TracingEnabled,
			Endpoint:    raw.TracingEndpoint,
			Insecure:    raw.TracingInsecure,
			SampleRatio: raw.TracingSampleRatio,
		},
	}, nil
}

func parseInfluxWriteMode(value string) (influxdbadapter.WriteMode, error) {
	mode := influxdbadapter.WriteMode(value)
	switch mode {
	case influxdbadapter.WriteModeBlocking, influxdbadapter.WriteModeBatch:
		return mode, nil
	default:
		return "", fmt.Errorf("INFLUX_WRITE_MODE must be one of %q or %q", influxdbadapter.WriteModeBlocking, influxdbadapter.WriteModeBatch)
	}
}

func readinessStaleness(pollInterval time.Duration) time.Duration {
	staleness := time.Duration(readinessMultiplier) * pollInterval
	if staleness < minReadinessStaleness {
		return minReadinessStaleness
	}

	return staleness
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
	case "LogLevel":
		return "LOG_LEVEL"
	case "AssetID":
		return "ASSET_ID"
	case "AssetType":
		return "ASSET_TYPE"
	case "ModbusHost":
		return "MODBUS_HOST"
	case "ModbusPort":
		return "MODBUS_PORT"
	case "ModbusUnitID":
		return "MODBUS_UNIT_ID"
	case "ModbusRegisterType":
		return "MODBUS_REGISTER_TYPE"
	case "ModbusSetpointAddress":
		return "MODBUS_SETPOINT_ADDRESS"
	case "ModbusActivePowerAddress":
		return "MODBUS_ACTIVE_POWER_ADDRESS"
	case "ModbusSignedValues":
		return "MODBUS_SIGNED_VALUES"
	case "PollInterval":
		return "POLL_INTERVAL"
	case "HTTPPort":
		return "HTTP_PORT"
	case "InfluxURL":
		return "INFLUX_URL"
	case "InfluxToken":
		return "INFLUX_TOKEN"
	case "InfluxOrg":
		return "INFLUX_ORG"
	case "InfluxBucket":
		return "INFLUX_BUCKET"
	case "InfluxWriteMode":
		return "INFLUX_WRITE_MODE"
	case "InfluxBatchSize":
		return "INFLUX_BATCH_SIZE"
	case "InfluxLogLevel":
		return "INFLUX_LOG_LEVEL"
	case "InfluxFlushInterval":
		return "INFLUX_FLUSH_INTERVAL"
	case "TracingEnabled":
		return "TRACING_ENABLED"
	case "TracingEndpoint":
		return "TRACING_ENDPOINT"
	case "TracingInsecure":
		return "TRACING_INSECURE"
	case "TracingSampleRatio":
		return "TRACING_SAMPLE_RATIO"
	default:
		return fieldName
	}
}
