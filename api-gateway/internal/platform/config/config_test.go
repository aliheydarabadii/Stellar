package config

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct {
	suite.Suite
}

func TestConfigSuite(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}

func (s *ConfigSuite) TestLoadDefaults() {
	s.T().Setenv("MEASUREMENT_SERVICE_GRPC_ADDR", "127.0.0.1:9090")
	s.T().Setenv("REDIS_ADDR", "127.0.0.1:6379")

	cfg, err := Load()

	s.Require().NoError(err)
	s.Equal(DefaultReadinessCheckTimeout, cfg.ReadinessCheckTimeout)
	s.Equal(DefaultHTTPReadHeaderTimeout, cfg.HTTPReadHeaderTimeout)
	s.Equal(DefaultHTTPReadTimeout, cfg.HTTPReadTimeout)
	s.Equal(DefaultHTTPWriteTimeout, cfg.HTTPWriteTimeout)
	s.Equal(DefaultHTTPIdleTimeout, cfg.HTTPIdleTimeout)
	s.Equal(DefaultHTTPMaxHeaderBytes, cfg.HTTPMaxHeaderBytes)
}

func (s *ConfigSuite) TestLoadRejectsInvalidReadinessTimeout() {
	s.T().Setenv("MEASUREMENT_SERVICE_GRPC_ADDR", "127.0.0.1:9090")
	s.T().Setenv("REDIS_ADDR", "127.0.0.1:6379")
	s.T().Setenv("READINESS_CHECK_TIMEOUT", "0s")

	_, err := Load()

	s.Require().Error(err)
	s.ErrorContains(err, "READINESS_CHECK_TIMEOUT must be positive")
}
