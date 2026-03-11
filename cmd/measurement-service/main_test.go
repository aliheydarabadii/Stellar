package main

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct {
	suite.Suite
}

func TestConfigSuite(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}

func (s *ConfigSuite) TestLoadConfigSuccess() {
	setValidEnv(s.T())
	s.T().Setenv("MAX_QUERY_RANGE", "20m")
	s.T().Setenv("QUERY_TIMEOUT", "15s")
	s.T().Setenv("INFLUX_CIRCUIT_BREAKER_FAILURE_THRESHOLD", "7")
	s.T().Setenv("INFLUX_CIRCUIT_BREAKER_OPEN_TIMEOUT", "45s")
	s.T().Setenv("INFLUX_CIRCUIT_BREAKER_HALF_OPEN_MAX_REQUESTS", "2")

	cfg, err := loadConfig()
	s.Require().NoError(err)

	s.Equal("http://localhost:8086", cfg.InfluxURL)
	s.Equal(20*time.Minute, cfg.MaxQueryRange)
	s.Equal(15*time.Second, cfg.QueryTimeout)
	s.Equal(7, cfg.InfluxCircuitBreakerFailureThreshold)
	s.Equal(45*time.Second, cfg.InfluxCircuitBreakerOpenTimeout)
	s.Equal(2, cfg.InfluxCircuitBreakerHalfOpenMaxRequests)
}

func (s *ConfigSuite) TestLoadConfigUsesDefaults() {
	setValidEnv(s.T())

	cfg, err := loadConfig()
	s.Require().NoError(err)

	s.Equal(":9090", cfg.GRPCListenAddr)
	s.Equal(":8080", cfg.HealthListenAddr)
	s.Equal(15*time.Minute, cfg.MaxQueryRange)
	s.Equal(10*time.Second, cfg.QueryTimeout)
}

func (s *ConfigSuite) TestLoadConfigFailsWhenRequiredInfluxEnvMissing() {
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
		s.Run(tc.name, func() {
			setValidEnv(s.T())
			s.T().Setenv(tc.key, "")

			_, err := loadConfig()
			s.Require().Error(err)
			s.Equal(tc.wantErr, err.Error())
		})
	}
}

func (s *ConfigSuite) TestLoadConfigFailsOnInvalidQueryTimeout() {
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
		s.Run(tc.name, func() {
			setValidEnv(s.T())
			s.T().Setenv("QUERY_TIMEOUT", tc.value)

			_, err := loadConfig()
			s.Require().Error(err)
			s.Contains(err.Error(), tc.want)
		})
	}
}

func (s *ConfigSuite) TestLoadConfigFailsOnInvalidMaxQueryRange() {
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
		s.Run(tc.name, func() {
			setValidEnv(s.T())
			s.T().Setenv("MAX_QUERY_RANGE", tc.value)

			_, err := loadConfig()
			s.Require().Error(err)
			s.Contains(err.Error(), tc.want)
		})
	}
}

func (s *ConfigSuite) TestLoadConfigFailsOnInvalidPositiveIntegerSettings() {
	testCases := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{
			name:  "failure threshold parse",
			key:   "INFLUX_CIRCUIT_BREAKER_FAILURE_THRESHOLD",
			value: "abc",
			want:  "parse INFLUX_CIRCUIT_BREAKER_FAILURE_THRESHOLD",
		},
		{
			name:  "failure threshold positive",
			key:   "INFLUX_CIRCUIT_BREAKER_FAILURE_THRESHOLD",
			value: "0",
			want:  "INFLUX_CIRCUIT_BREAKER_FAILURE_THRESHOLD must be positive",
		},
		{
			name:  "half open requests parse",
			key:   "INFLUX_CIRCUIT_BREAKER_HALF_OPEN_MAX_REQUESTS",
			value: "abc",
			want:  "parse INFLUX_CIRCUIT_BREAKER_HALF_OPEN_MAX_REQUESTS",
		},
		{
			name:  "half open requests positive",
			key:   "INFLUX_CIRCUIT_BREAKER_HALF_OPEN_MAX_REQUESTS",
			value: "-1",
			want:  "INFLUX_CIRCUIT_BREAKER_HALF_OPEN_MAX_REQUESTS must be positive",
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			setValidEnv(s.T())
			s.T().Setenv(tc.key, tc.value)

			_, err := loadConfig()
			s.Require().Error(err)
			s.True(strings.Contains(err.Error(), tc.want), "expected error containing %q, got %v", tc.want, err)
		})
	}
}

func setValidEnv(t *testing.T) {
	t.Helper()

	t.Setenv("INFLUX_URL", "http://localhost:8086")
	t.Setenv("INFLUX_ORG", "acme")
	t.Setenv("INFLUX_BUCKET", "measurements")
	t.Setenv("INFLUX_TOKEN", "secret")
}
