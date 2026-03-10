package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"stellar/internal/telemetry/adapters/influxdb"
	"stellar/internal/telemetry/adapters/modbus"
	"stellar/internal/telemetry/app"
	"stellar/internal/telemetry/domain"
	"stellar/internal/telemetry/ports"
)

const (
	serviceName          = "integration-service"
	defaultInfluxTimeout = 5 * time.Second
)

type config struct {
	AssetID      domain.AssetID
	AssetType    domain.AssetType
	PollInterval time.Duration
	HTTPPort     int
	Modbus       modbus.Config
	Influx       influxdb.Config
}

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

func run(ctx context.Context, cfg config, logger *slog.Logger) error {
	addressMapper := modbus.NewAddressMapper()
	decoder := modbus.NewDecoder()

	source, err := modbus.NewSource(cfg.Modbus, addressMapper, decoder)
	if err != nil {
		return fmt.Errorf("create modbus source: %w", err)
	}

	pointMapper := influxdb.NewPointMapperWithAssetType(string(cfg.AssetType))
	repository, err := influxdb.NewMeasurementRepositoryWithConfig(cfg.Influx, pointMapper)
	if err != nil {
		return fmt.Errorf("create influxdb repository: %w", err)
	}
	defer repository.Close()

	application := app.NewApplication(cfg.AssetID, source, repository)

	worker, err := ports.NewTickerWorker(cfg.PollInterval, application.Commands.CollectTelemetry, logger)
	if err != nil {
		return fmt.Errorf("create worker: %w", err)
	}

	httpServer, err := ports.NewHTTPServer(httpAddress(cfg.HTTPPort), logger)
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
	)

	return runComponents(ctx, logger, httpServer, worker)
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

func loadConfig() (config, error) {
	assetID, err := requiredAssetID("ASSET_ID")
	if err != nil {
		return config{}, err
	}

	assetType, err := requiredAssetType("ASSET_TYPE")
	if err != nil {
		return config{}, err
	}

	modbusHost, err := requiredString("MODBUS_HOST")
	if err != nil {
		return config{}, err
	}

	modbusPort, err := requiredUint16("MODBUS_PORT")
	if err != nil {
		return config{}, err
	}

	unitID, err := requiredUint8("MODBUS_UNIT_ID")
	if err != nil {
		return config{}, err
	}

	registerTypeValue, err := requiredString("MODBUS_REGISTER_TYPE")
	if err != nil {
		return config{}, err
	}

	setpointAddress, err := requiredUint16("MODBUS_SETPOINT_ADDRESS")
	if err != nil {
		return config{}, err
	}

	activePowerAddress, err := requiredUint16("MODBUS_ACTIVE_POWER_ADDRESS")
	if err != nil {
		return config{}, err
	}

	signedValues, err := requiredBool("MODBUS_SIGNED_VALUES")
	if err != nil {
		return config{}, err
	}

	pollInterval, err := requiredDuration("POLL_INTERVAL")
	if err != nil {
		return config{}, err
	}

	httpPort, err := requiredPort("HTTP_PORT")
	if err != nil {
		return config{}, err
	}

	influxURL, err := requiredString("INFLUX_URL")
	if err != nil {
		return config{}, err
	}

	influxToken, err := requiredString("INFLUX_TOKEN")
	if err != nil {
		return config{}, err
	}

	influxOrg, err := requiredString("INFLUX_ORG")
	if err != nil {
		return config{}, err
	}

	influxBucket, err := requiredString("INFLUX_BUCKET")
	if err != nil {
		return config{}, err
	}

	registerMapping, err := domain.NewRegisterMapping(
		domain.RegisterType(registerTypeValue),
		setpointAddress,
		activePowerAddress,
		signedValues,
	)
	if err != nil {
		return config{}, fmt.Errorf("build register mapping: %w", err)
	}

	return config{
		AssetID:      assetID,
		AssetType:    assetType,
		PollInterval: pollInterval,
		HTTPPort:     httpPort,
		Modbus: modbus.Config{
			Host:            modbusHost,
			Port:            modbusPort,
			UnitID:          unitID,
			RegisterMapping: registerMapping,
		},
		Influx: influxdb.Config{
			BaseURL: influxURL,
			Org:     influxOrg,
			Bucket:  influxBucket,
			Token:   influxToken,
			Timeout: defaultInfluxTimeout,
		},
	}, nil
}

func requiredString(key string) (string, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("missing required environment variable %s", key)
	}

	return strings.TrimSpace(value), nil
}

func requiredAssetID(key string) (domain.AssetID, error) {
	value, err := requiredString(key)
	if err != nil {
		return "", err
	}

	return domain.AssetID(value), nil
}

func requiredAssetType(key string) (domain.AssetType, error) {
	value, err := requiredString(key)
	if err != nil {
		return "", err
	}

	return domain.AssetType(value), nil
}

func requiredUint16(key string) (uint16, error) {
	value, err := requiredString(key)
	if err != nil {
		return 0, err
	}

	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("parse %s as uint16: %w", key, err)
	}

	return uint16(parsed), nil
}

func requiredUint8(key string) (uint8, error) {
	value, err := requiredString(key)
	if err != nil {
		return 0, err
	}

	parsed, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return 0, fmt.Errorf("parse %s as uint8: %w", key, err)
	}

	return uint8(parsed), nil
}

func requiredBool(key string) (bool, error) {
	value, err := requiredString(key)
	if err != nil {
		return false, err
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s as bool: %w", key, err)
	}

	return parsed, nil
}

func requiredDuration(key string) (time.Duration, error) {
	value, err := requiredString(key)
	if err != nil {
		return 0, err
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s as duration: %w", key, err)
	}

	if duration <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}

	return duration, nil
}

func requiredPort(key string) (int, error) {
	value, err := requiredString(key)
	if err != nil {
		return 0, err
	}

	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s as port: %w", key, err)
	}

	if port <= 0 || port > 65535 {
		return 0, fmt.Errorf("%s must be between 1 and 65535", key)
	}

	return port, nil
}

func httpAddress(port int) string {
	return ":" + strconv.Itoa(port)
}
