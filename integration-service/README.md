# Integration Service

The Integration Service is the command-side telemetry ingester for the system.

It polls Modbus TCP registers, converts raw values into telemetry readings, validates them against domain rules, and persists valid measurements to InfluxDB. It also exposes operational health/readiness endpoints, Prometheus metrics, and OpenTelemetry traces.

## Architecture

This service is intentionally lightweight:

- command-side CQRS only
- clean-architecture / hexagonal-friendly package boundaries
- pragmatic, lightweight DDD rather than heavy domain modeling

The ownership model is:

- `internal/telemetry`: feature types and validation rules
- `internal/telemetry/application`: `CollectTelemetryHandler` and the collection command DTO
- `internal/telemetry/ports.go`: feature-level source and repository ports plus the raw `TelemetryReading` contract
- `internal/telemetry/mocks`: mockery-style test doubles for feature ports
- `adapters/inbound/worker`: the polling loop that triggers collection
- `adapters/outbound/modbus`: Modbus TCP telemetry source
- `adapters/outbound/influxdb`: InfluxDB measurement repository
- `platform`: operational concerns such as config, logging, health, metrics, and tracing
- `cmd/integration-service`: composition root and process bootstrap

This service does not expose query endpoints and does not own the API Gateway freshness cache.

## Package Structure

```text
cmd/
  integration-service/
    main.go

internal/
  telemetry/
    asset.go
    register_mapping.go
    measurement.go
    errors.go
    ports.go
    mocks/
      MeasurementRepository.go
      TelemetrySource.go

    application/
      command.go
      command_handler.go
      command_handler_test.go
      errors.go

    adapters/
      inbound/
        worker/
          runner.go
      outbound/
        modbus/
          source.go
          decoder.go
          address_mapper.go
        influxdb/
          measurement_repository.go
          point_mapper.go

  platform/
    config/
      config.go
    logging/
      logger.go
    health/
      handler.go
      readiness.go
    metrics/
      metrics.go
      instrumentation.go
    tracing/
      tracing.go
```

## Runtime Flow

1. The worker wakes up on the configured poll interval.
2. It creates `CollectTelemetry{CollectedAt: time.Now().UTC()}`.
3. `CollectTelemetryHandler` asks the Modbus source for raw telemetry.
4. The application builds a domain `Measurement`.
5. Invalid measurements are rejected and skipped.
6. Valid measurements are written to InfluxDB.
7. Metrics, readiness, and traces are emitted around the collection path.

## Configuration

The service reads configuration from environment variables.

Required:

```text
ASSET_ID
ASSET_TYPE
MODBUS_HOST
MODBUS_PORT
MODBUS_UNIT_ID
MODBUS_REGISTER_TYPE
MODBUS_SETPOINT_ADDRESS
MODBUS_ACTIVE_POWER_ADDRESS
MODBUS_SIGNED_VALUES
POLL_INTERVAL
INFLUX_URL
INFLUX_TOKEN
INFLUX_ORG
INFLUX_BUCKET
HTTP_PORT
```

Optional:

```text
LOG_LEVEL
INFLUX_LOG_LEVEL
INFLUX_WRITE_MODE
INFLUX_BATCH_SIZE
INFLUX_FLUSH_INTERVAL
TRACING_ENABLED
TRACING_ENDPOINT
TRACING_INSECURE
TRACING_SAMPLE_RATIO
```

Notes:

- `LOG_LEVEL` defaults to `INFO`
- `INFLUX_WRITE_MODE` supports `blocking` and `batch`
- `INFLUX_LOG_LEVEL` maps directly to the InfluxDB Go client log level (`uint`)
- readiness becomes unhealthy if successful collections go stale beyond the derived readiness window

Example:

```bash
export LOG_LEVEL=INFO
export ASSET_ID=871689260010377213
export ASSET_TYPE=solar_panel
export MODBUS_HOST=127.0.0.1
export MODBUS_PORT=5020
export MODBUS_UNIT_ID=1
export MODBUS_REGISTER_TYPE=holding
export MODBUS_SETPOINT_ADDRESS=40100
export MODBUS_ACTIVE_POWER_ADDRESS=40101
export MODBUS_SIGNED_VALUES=true
export POLL_INTERVAL=1s
export INFLUX_URL=http://127.0.0.1:8086
export INFLUX_TOKEN=dev-token
export INFLUX_ORG=local
export INFLUX_BUCKET=telemetry
export INFLUX_LOG_LEVEL=0
export INFLUX_WRITE_MODE=blocking
export TRACING_ENABLED=false
export TRACING_ENDPOINT=http://127.0.0.1:4318
export TRACING_INSECURE=true
export TRACING_SAMPLE_RATIO=1.0
export HTTP_PORT=8080
```

## Run

```bash
go run ./cmd/integration-service
```

Or:

```bash
make run-local
```

Endpoints:

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

Readiness behavior:

- `/healthz` reports that the process is running
- `/readyz` becomes healthy only after at least one successful end-to-end collection and persistence cycle
- `/readyz` turns unhealthy during shutdown or after the success window goes stale

Trace spans emitted by the collection path:

- `telemetry.collect`
- `telemetry.source.read`
- `telemetry.persistence.save`

## Docker Compose

Run the full local stack with:

```bash
docker compose up --build
```

Or:

```bash
make compose-up
```

Services started:

- `integration-service`
- `influxdb`
- `modbus-server`
- `prometheus`

Exposed ports:

- `8080`: Integration Service HTTP endpoints
- `8086`: InfluxDB
- `5020`: Modbus server
- `9090`: Prometheus UI

The compose stack uses the mounted Modbus config at `docker/modbus/server_config.json`.

## Testing

Run the Go test suite with:

```bash
go test ./...
```

The test suite covers domain validation, application orchestration, Modbus decoding/source behavior, InfluxDB repository behavior, health/readiness handling, metrics instrumentation, and the worker loop.

## Load Testing

The service does not expose a public ingestion API, so the provided k6 scripts cover:

- `/healthz` and `/readyz` HTTP surface load
- direct InfluxDB write load for repository throughput experiments

Run the HTTP load test:

```bash
k6 run loadtest/k6/service-http.js
```

Run the Influx write load test:

```bash
INFLUX_URL=http://localhost:8086 \
INFLUX_ORG=local \
INFLUX_BUCKET=telemetry \
INFLUX_TOKEN=dev-token \
k6 run loadtest/k6/influx-write.js
```
