package main

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"api_gateway/internal/platform/config"
	"github.com/stretchr/testify/suite"
)

type MainSuite struct {
	suite.Suite
}

func TestMainSuite(t *testing.T) {
	suite.Run(t, new(MainSuite))
}

func (s *MainSuite) TestNewReadinessCheckFailsWhenServiceNotStarted() {
	var started atomic.Bool

	check := newReadinessCheck(&started, readinessDependencyFunc(func(context.Context) error {
		return nil
	}))

	err := check(context.Background())

	s.Require().Error(err)
}

func (s *MainSuite) TestNewReadinessCheckCallsDependencies() {
	var started atomic.Bool
	started.Store(true)

	called := false
	check := newReadinessCheck(&started, readinessDependencyFunc(func(context.Context) error {
		called = true
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := check(ctx)

	s.Require().NoError(err)
	s.True(called)
}

func (s *MainSuite) TestNewHTTPServerUsesConfiguredTimeouts() {
	cfg := config.Config{
		HTTPReadHeaderTimeout: 5 * time.Second,
		HTTPReadTimeout:       10 * time.Second,
		HTTPWriteTimeout:      15 * time.Second,
		HTTPIdleTimeout:       60 * time.Second,
		HTTPMaxHeaderBytes:    1 << 20,
	}

	server := newHTTPServer(":8080", http.NewServeMux(), cfg)

	s.Equal(cfg.HTTPReadHeaderTimeout, server.ReadHeaderTimeout)
	s.Equal(cfg.HTTPReadTimeout, server.ReadTimeout)
	s.Equal(cfg.HTTPWriteTimeout, server.WriteTimeout)
	s.Equal(cfg.HTTPIdleTimeout, server.IdleTimeout)
	s.Equal(cfg.HTTPMaxHeaderBytes, server.MaxHeaderBytes)
}

type readinessDependencyFunc func(context.Context) error

func (f readinessDependencyFunc) Ready(ctx context.Context) error {
	return f(ctx)
}
