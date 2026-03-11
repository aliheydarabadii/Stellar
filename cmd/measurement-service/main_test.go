package main

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigSuccess(t *testing.T) {
	setValidEnv(t)
	t.Setenv("MAX_QUERY_RANGE", "20m")
	t.Setenv("QUERY_TIMEOUT", "15s")
	t.Setenv("INFLUX_CIRCUIT_BREAKER_FAILURE_THRESHOLD", "7")
	t.Setenv("INFLUX_CIRCUIT_BREAKER_OPEN_TIMEOUT", "45s")
	t.Setenv("INFLUX_CIRCUIT_BREAKER_HALF_OPEN_MAX_REQUESTS", "2")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.InfluxURL != "http://localhost:8086" {
		t.Fatalf("expected influx url to be set, got %q", cfg.InfluxURL)
	}
	if cfg.MaxQueryRange != 20*time.Minute {
		t.Fatalf("expected max query range 20m, got %v", cfg.MaxQueryRange)
	}
	if cfg.QueryTimeout != 15*time.Second {
		t.Fatalf("expected query timeout 15s, got %v", cfg.QueryTimeout)
	}
	if cfg.InfluxCircuitBreakerFailureThreshold != 7 {
		t.Fatalf("expected breaker failure threshold 7, got %d", cfg.InfluxCircuitBreakerFailureThreshold)
	}
	if cfg.InfluxCircuitBreakerOpenTimeout != 45*time.Second {
		t.Fatalf("expected breaker open timeout 45s, got %v", cfg.InfluxCircuitBreakerOpenTimeout)
	}
	if cfg.InfluxCircuitBreakerHalfOpenMaxRequests != 2 {
		t.Fatalf("expected breaker half-open max requests 2, got %d", cfg.InfluxCircuitBreakerHalfOpenMaxRequests)
	}
}

func TestLoadConfigUsesDefaultMaxQueryRange(t *testing.T) {
	setValidEnv(t)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.MaxQueryRange != 15*time.Minute {
		t.Fatalf("expected default max query range 15m, got %v", cfg.MaxQueryRange)
	}
}

func TestLoadConfigFailsWhenRequiredInfluxEnvMissing(t *testing.T) {
	testCases := []struct {
		name    string
		key     string
		wantErr string
	}{
		{name: "url", key: "INFLUX_URL", wantErr: "INFLUX_URL is required"},
		{name: "org", key: "INFLUX_ORG", wantErr: "INFLUX_ORG is required"},
		{name: "bucket", key: "INFLUX_BUCKET", wantErr: "INFLUX_BUCKET is required"},
		{name: "token", key: "INFLUX_TOKEN", wantErr: "INFLUX_TOKEN is required"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv(tc.key, "")

			_, err := loadConfig()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("expected %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestLoadConfigFailsOnInvalidQueryTimeout(t *testing.T) {
	testCases := []struct {
		name  string
		value string
		want  string
	}{
		{name: "invalid duration", value: "not-a-duration", want: "parse QUERY_TIMEOUT"},
		{name: "zero duration", value: "0s", want: "QUERY_TIMEOUT must be positive"},
		{name: "negative duration", value: "-5s", want: "QUERY_TIMEOUT must be positive"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("QUERY_TIMEOUT", tc.value)

			_, err := loadConfig()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestLoadConfigFailsOnInvalidMaxQueryRange(t *testing.T) {
	testCases := []struct {
		name  string
		value string
		want  string
	}{
		{name: "invalid duration", value: "not-a-duration", want: "parse MAX_QUERY_RANGE"},
		{name: "non-positive duration", value: "0s", want: "MAX_QUERY_RANGE must be positive"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("MAX_QUERY_RANGE", tc.value)

			_, err := loadConfig()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestParsePositiveInt(t *testing.T) {
	value, err := parsePositiveInt("3")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if value != 3 {
		t.Fatalf("expected 3, got %d", value)
	}
}

func TestParsePositiveIntRejectsInvalidValues(t *testing.T) {
	testCases := []string{"0", "-1", "abc"}

	for _, input := range testCases {
		input := input
		t.Run(input, func(t *testing.T) {
			_, err := parsePositiveInt(input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func setValidEnv(t *testing.T) {
	t.Helper()

	t.Setenv("INFLUX_URL", "http://localhost:8086")
	t.Setenv("INFLUX_ORG", "acme")
	t.Setenv("INFLUX_BUCKET", "measurements")
	t.Setenv("INFLUX_TOKEN", "secret")
	t.Setenv("GRPC_LISTEN_ADDR", ":9090")
	t.Setenv("HEALTH_LISTEN_ADDR", ":8080")
}
