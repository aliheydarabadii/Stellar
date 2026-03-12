package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("ASSET_ID", "871689260010377213")
	t.Setenv("ASSET_TYPE", "solar_panel")
	t.Setenv("MODBUS_HOST", "127.0.0.1")
	t.Setenv("MODBUS_PORT", "5020")
	t.Setenv("MODBUS_UNIT_ID", "1")
	t.Setenv("MODBUS_REGISTER_TYPE", "holding")
	t.Setenv("MODBUS_SETPOINT_ADDRESS", "40100")
	t.Setenv("MODBUS_ACTIVE_POWER_ADDRESS", "40101")
	t.Setenv("MODBUS_SIGNED_VALUES", "true")
	t.Setenv("POLL_INTERVAL", "1s")
	t.Setenv("HTTP_PORT", "8080")
	t.Setenv("INFLUX_URL", "http://localhost:8086")
	t.Setenv("INFLUX_TOKEN", "dev-token")
	t.Setenv("INFLUX_ORG", "local")
	t.Setenv("INFLUX_BUCKET", "telemetry")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LogLevel != "INFO" {
		t.Fatalf("expected default LOG_LEVEL to be INFO, got %q", cfg.LogLevel)
	}
	if cfg.PollInterval != time.Second {
		t.Fatalf("expected poll interval 1s, got %s", cfg.PollInterval)
	}
	if cfg.ReadinessStaleAfter != 5*time.Second {
		t.Fatalf("expected readiness stale window 5s, got %s", cfg.ReadinessStaleAfter)
	}
	if cfg.HTTPPort != 8080 {
		t.Fatalf("expected HTTP port 8080, got %d", cfg.HTTPPort)
	}
}

func TestLoadSupportsConfiguredLogLevel(t *testing.T) {
	t.Setenv("ASSET_ID", "871689260010377213")
	t.Setenv("ASSET_TYPE", "solar_panel")
	t.Setenv("MODBUS_HOST", "127.0.0.1")
	t.Setenv("MODBUS_PORT", "5020")
	t.Setenv("MODBUS_UNIT_ID", "1")
	t.Setenv("MODBUS_REGISTER_TYPE", "holding")
	t.Setenv("MODBUS_SETPOINT_ADDRESS", "40100")
	t.Setenv("MODBUS_ACTIVE_POWER_ADDRESS", "40101")
	t.Setenv("MODBUS_SIGNED_VALUES", "true")
	t.Setenv("POLL_INTERVAL", "2s")
	t.Setenv("HTTP_PORT", "8080")
	t.Setenv("INFLUX_URL", "http://localhost:8086")
	t.Setenv("INFLUX_TOKEN", "dev-token")
	t.Setenv("INFLUX_ORG", "local")
	t.Setenv("INFLUX_BUCKET", "telemetry")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Fatalf("expected LOG_LEVEL debug, got %q", cfg.LogLevel)
	}
}

func TestLoadRejectsInvalidHTTPPort(t *testing.T) {
	t.Setenv("ASSET_ID", "871689260010377213")
	t.Setenv("ASSET_TYPE", "solar_panel")
	t.Setenv("MODBUS_HOST", "127.0.0.1")
	t.Setenv("MODBUS_PORT", "5020")
	t.Setenv("MODBUS_UNIT_ID", "1")
	t.Setenv("MODBUS_REGISTER_TYPE", "holding")
	t.Setenv("MODBUS_SETPOINT_ADDRESS", "40100")
	t.Setenv("MODBUS_ACTIVE_POWER_ADDRESS", "40101")
	t.Setenv("MODBUS_SIGNED_VALUES", "true")
	t.Setenv("POLL_INTERVAL", "1s")
	t.Setenv("HTTP_PORT", "70000")
	t.Setenv("INFLUX_URL", "http://localhost:8086")
	t.Setenv("INFLUX_TOKEN", "dev-token")
	t.Setenv("INFLUX_ORG", "local")
	t.Setenv("INFLUX_BUCKET", "telemetry")

	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid HTTP_PORT error")
	}

	if err.Error() != "HTTP_PORT must be between 1 and 65535" {
		t.Fatalf("unexpected error: %v", err)
	}
}
