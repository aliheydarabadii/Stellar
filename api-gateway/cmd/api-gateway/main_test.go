package main

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("MEASUREMENT_SERVICE_GRPC_ADDR", "127.0.0.1:9090")
	t.Setenv("REDIS_ADDR", "127.0.0.1:6379")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ReadinessCheckTimeout != defaultReadinessCheckTimeout {
		t.Fatalf("expected readiness timeout %v, got %v", defaultReadinessCheckTimeout, cfg.ReadinessCheckTimeout)
	}
	if cfg.HTTPReadHeaderTimeout != defaultHTTPReadHeaderTimeout {
		t.Fatalf("expected read header timeout %v, got %v", defaultHTTPReadHeaderTimeout, cfg.HTTPReadHeaderTimeout)
	}
	if cfg.HTTPReadTimeout != defaultHTTPReadTimeout {
		t.Fatalf("expected read timeout %v, got %v", defaultHTTPReadTimeout, cfg.HTTPReadTimeout)
	}
	if cfg.HTTPWriteTimeout != defaultHTTPWriteTimeout {
		t.Fatalf("expected write timeout %v, got %v", defaultHTTPWriteTimeout, cfg.HTTPWriteTimeout)
	}
	if cfg.HTTPIdleTimeout != defaultHTTPIdleTimeout {
		t.Fatalf("expected idle timeout %v, got %v", defaultHTTPIdleTimeout, cfg.HTTPIdleTimeout)
	}
	if cfg.HTTPMaxHeaderBytes != defaultHTTPMaxHeaderBytes {
		t.Fatalf("expected max header bytes %d, got %d", defaultHTTPMaxHeaderBytes, cfg.HTTPMaxHeaderBytes)
	}
}

func TestLoadConfigRejectsInvalidReadinessTimeout(t *testing.T) {
	t.Setenv("MEASUREMENT_SERVICE_GRPC_ADDR", "127.0.0.1:9090")
	t.Setenv("REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("READINESS_CHECK_TIMEOUT", "0s")

	_, err := loadConfig()
	if err == nil || !strings.Contains(err.Error(), "READINESS_CHECK_TIMEOUT must be positive") {
		t.Fatalf("expected readiness timeout validation error, got %v", err)
	}
}

func TestNewReadinessCheckFailsWhenServiceNotStarted(t *testing.T) {
	var started atomic.Bool

	check := newReadinessCheck(&started, readinessDependencyFunc(func(context.Context) error {
		return nil
	}))

	if err := check(context.Background()); err == nil {
		t.Fatal("expected readiness check to fail before startup")
	}
}

func TestNewReadinessCheckCallsDependencies(t *testing.T) {
	var started atomic.Bool
	started.Store(true)

	called := false
	check := newReadinessCheck(&started, readinessDependencyFunc(func(context.Context) error {
		called = true
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := check(ctx); err != nil {
		t.Fatalf("expected readiness check to succeed, got %v", err)
	}
	if !called {
		t.Fatal("expected dependency to be called")
	}
}

type readinessDependencyFunc func(context.Context) error

func (f readinessDependencyFunc) Ready(ctx context.Context) error {
	return f(ctx)
}
