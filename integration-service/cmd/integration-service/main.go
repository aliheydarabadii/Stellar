package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"stellar/internal/telemetry/adapters/influxdb"
	"stellar/internal/telemetry/adapters/modbus"
	"stellar/internal/telemetry/app"
	"stellar/internal/telemetry/ports"
)

const (
	serviceName          = "integration-service"
	defaultTraceShutdown = 5 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})).With("service", serviceName)
	slog.SetDefault(logger)

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("service stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config, logger *slog.Logger) (runErr error) {
	shutdownTracing, err := ports.SetupTracing(ctx, serviceName, cfg.Tracing)
	if err != nil {
		return fmt.Errorf("setup tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultTraceShutdown)
		defer cancel()

		if err := shutdownTracing(shutdownCtx); err != nil {
			logger.Error("failed to shut down tracing", "error", err)
		}
	}()

	addressMapper := modbus.NewAddressMapper()
	decoder := modbus.NewDecoder()
	metrics := ports.NewMetrics()
	tracer := otel.Tracer(serviceName)
	readiness, err := ports.NewReadiness(cfg.ReadinessStaleAfter)
	if err != nil {
		return fmt.Errorf("create readiness: %w", err)
	}

	source, err := modbus.NewSource(cfg.Modbus, addressMapper, decoder)
	if err != nil {
		return fmt.Errorf("create modbus source: %w", err)
	}
	instrumentedSource := ports.InstrumentTelemetrySource(source, metrics, tracer)

	pointMapper := influxdb.NewPointMapperWithAssetType(string(cfg.AssetType))
	repository, err := influxdb.NewMeasurementRepositoryWithConfig(cfg.Influx, pointMapper)
	if err != nil {
		return fmt.Errorf("create influxdb repository: %w", err)
	}
	defer func() {
		if err := repository.Close(); err != nil {
			if runErr == nil {
				runErr = fmt.Errorf("close influxdb repository: %w", err)
				return
			}

			logger.Error("failed to close influxdb repository", "error", err)
		}
	}()
	instrumentedRepository := ports.InstrumentMeasurementRepository(repository, metrics, tracer)

	application, err := app.NewApplication(cfg.AssetID, instrumentedSource, instrumentedRepository)
	if err != nil {
		return fmt.Errorf("create application: %w", err)
	}

	worker, err := ports.NewTickerWorker(cfg.PollInterval, application.Commands.CollectTelemetry, logger, metrics, readiness, tracer)
	if err != nil {
		return fmt.Errorf("create worker: %w", err)
	}

	httpServer, err := ports.NewHTTPServer(httpAddress(cfg.HTTPPort), logger, metrics, readiness)
	if err != nil {
		return fmt.Errorf("create http server: %w", err)
	}

	logger.Info(
		"service starting",
		"asset_id", cfg.AssetID.String(),
		"asset_type", string(cfg.AssetType),
		"modbus_host", cfg.Modbus.Host,
		"modbus_port", cfg.Modbus.Port,
		"http_port", cfg.HTTPPort,
		"poll_interval", cfg.PollInterval.String(),
		"influx_url", cfg.Influx.BaseURL,
		"influx_log_level", cfg.Influx.LogLevel,
		"influx_write_mode", string(cfg.Influx.WriteMode),
		"tracing_enabled", cfg.Tracing.Enabled,
		"tracing_endpoint", cfg.Tracing.Endpoint,
	)

	runErr = runComponents(ctx, logger, httpServer, worker)
	return runErr
}

func runComponents(ctx context.Context, logger *slog.Logger, httpServer ports.HTTPServer, worker ports.Worker) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	start := func(name string, fn func(context.Context) error) {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := fn(ctx); err != nil && !errors.Is(err, context.Canceled) {
				select {
				case errCh <- fmt.Errorf("%s: %w", name, err):
				default:
				}
				cancel()
			}
		}()
	}

	start("http server", httpServer.Start)
	start("worker", worker.Start)

	var runErr error

	select {
	case <-ctx.Done():
	case err := <-errCh:
		runErr = err
	}

	cancel()
	wg.Wait()

	if runErr != nil {
		return runErr
	}

	logger.Info("service stopped")
	return nil
}

func httpAddress(port int) string {
	return ":" + strconv.Itoa(port)
}
