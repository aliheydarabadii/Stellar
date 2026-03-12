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
	configplatform "stellar/internal/platform/config"
	healthplatform "stellar/internal/platform/health"
	loggingplatform "stellar/internal/platform/logging"
	metricsplatform "stellar/internal/platform/metrics"
	tracingplatform "stellar/internal/platform/tracing"
	workeradapter "stellar/internal/telemetry/adapters/inbound/worker"
	influxdbadapter "stellar/internal/telemetry/adapters/outbound/influxdb"
	modbusadapter "stellar/internal/telemetry/adapters/outbound/modbus"
	collecttelemetry "stellar/internal/telemetry/application"
)

const (
	serviceName          = "integration-service"
	defaultTraceShutdown = 5 * time.Second
)

func main() {
	bootstrapLogger := loggingplatform.NewDefaultLogger().With("service", serviceName)

	cfg, err := configplatform.Load()
	if err != nil {
		bootstrapLogger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger, err := loggingplatform.NewLogger(cfg.LogLevel)
	if err != nil {
		bootstrapLogger.Error("failed to build logger", "error", err)
		os.Exit(1)
	}
	logger = logger.With("service", serviceName)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("service stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg configplatform.Config, logger *slog.Logger) (runErr error) {
	shutdownTracing, err := tracingplatform.SetupTracing(ctx, serviceName, cfg.Tracing)
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

	addressMapper := modbusadapter.NewAddressMapper()
	decoder := modbusadapter.NewDecoder()
	metrics := metricsplatform.NewMetrics()
	tracer := otel.Tracer(serviceName)
	readiness, err := healthplatform.NewReadiness(cfg.ReadinessStaleAfter)
	if err != nil {
		return fmt.Errorf("create readiness: %w", err)
	}

	modbusConfig := buildModbusConfig(cfg)
	source, err := modbusadapter.NewSource(modbusConfig, addressMapper, decoder)
	if err != nil {
		return fmt.Errorf("create modbus source: %w", err)
	}
	instrumentedSource := metricsplatform.InstrumentTelemetrySource(source, metrics, tracer)

	pointMapper := influxdbadapter.NewPointMapperWithAssetType(string(cfg.AssetType))
	influxConfig, err := buildInfluxConfig(cfg)
	if err != nil {
		return fmt.Errorf("build influxdb config: %w", err)
	}
	repository, err := influxdbadapter.NewMeasurementRepositoryWithConfig(influxConfig, pointMapper)
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
	instrumentedRepository := metricsplatform.InstrumentMeasurementRepository(repository, metrics, tracer)

	collectTelemetry, err := collecttelemetry.NewCollectTelemetryHandler(cfg.AssetID, instrumentedSource, instrumentedRepository)
	if err != nil {
		return fmt.Errorf("create collect telemetry handler: %w", err)
	}

	worker, err := workeradapter.NewRunner(cfg.PollInterval, collectTelemetry.Handle, logger, metrics, readiness, tracer)
	if err != nil {
		return fmt.Errorf("create worker: %w", err)
	}

	httpServer, err := healthplatform.NewServer(httpAddress(cfg.HTTPPort), logger, metrics, readiness)
	if err != nil {
		return fmt.Errorf("create http server: %w", err)
	}

	logger.Info(
		"service starting",
		"asset_id", cfg.AssetID.String(),
		"asset_type", string(cfg.AssetType),
		"modbus_host", modbusConfig.Host,
		"modbus_port", modbusConfig.Port,
		"http_port", cfg.HTTPPort,
		"poll_interval", cfg.PollInterval.String(),
		"influx_url", influxConfig.BaseURL,
		"influx_log_level", influxConfig.LogLevel,
		"influx_write_mode", string(influxConfig.WriteMode),
		"tracing_enabled", cfg.Tracing.Enabled,
		"tracing_endpoint", cfg.Tracing.Endpoint,
	)

	runErr = runComponents(ctx, logger, httpServer, worker)
	return runErr
}

func runComponents(ctx context.Context, logger *slog.Logger, httpServer healthplatform.HTTPServer, worker workeradapter.Worker) error {
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

func buildModbusConfig(cfg configplatform.Config) modbusadapter.Config {
	return modbusadapter.Config{
		Host:            cfg.Modbus.Host,
		Port:            cfg.Modbus.Port,
		UnitID:          cfg.Modbus.UnitID,
		RegisterMapping: cfg.Modbus.RegisterMapping,
	}
}

func buildInfluxConfig(cfg configplatform.Config) (influxdbadapter.Config, error) {
	writeMode, err := parseInfluxWriteMode(cfg.Influx.WriteMode)
	if err != nil {
		return influxdbadapter.Config{}, err
	}

	return influxdbadapter.Config{
		BaseURL:       cfg.Influx.BaseURL,
		Org:           cfg.Influx.Org,
		Bucket:        cfg.Influx.Bucket,
		Token:         cfg.Influx.Token,
		Timeout:       cfg.Influx.Timeout,
		LogLevel:      cfg.Influx.LogLevel,
		WriteMode:     writeMode,
		BatchSize:     cfg.Influx.BatchSize,
		FlushInterval: cfg.Influx.FlushInterval,
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
