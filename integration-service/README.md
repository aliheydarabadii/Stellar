# Integration Service

The Integration Service is the command-side telemetry ingester for the backend system.

It polls Modbus TCP registers, converts raw values into telemetry readings, validates domain measurements, and writes valid measurements to InfluxDB.

## Purpose

- collect telemetry for a configured asset
- validate telemetry using domain rules before persistence
- persist valid measurements to InfluxDB
- expose minimal health and readiness endpoints for runtime supervision
- expose Prometheus metrics for service monitoring
- export OpenTelemetry traces for collection, source, and persistence spans

## Architecture

This service follows a lightweight DDD + CQRS + Clean Architecture style:

- `domain`: business concepts and validation rules
- `app`: command-side orchestration
- `adapters`: infrastructure implementations for Modbus and InfluxDB
- `ports`: runtime concerns such as worker execution and HTTP endpoints
- `cmd/integration-service`: bootstrap and wiring

CQRS note:

- this service is command-side only
- there are no query handlers or read-side endpoints in this service

## Package Structure

```text
cmd/
  integration-service/
    main.go

internal/
  telemetry/
    app/
      app.go
      command/
        collect_telemetry.go

    domain/
      asset.go
      register_mapping.go
      measurement.go
      errors.go

    ports/
      worker.go
      http.go

    adapters/
      modbus/
        source.go
        decoder.go
        address_mapper.go
      influxdb/
        measurement_repository.go
        point_mapper.go
```

## Runtime Flow

1. A ticker triggers telemetry collection on the configured poll interval.
2. The worker builds `CollectTelemetry{CollectedAt: time.Now().UTC()}`.
3. The command handler reads raw telemetry from the Modbus adapter.
4. The application constructs a domain `Measurement`.
5. Invalid measurements are rejected and skipped.
6. Valid measurements are persisted through the InfluxDB adapter.

## Assumptions

- time is passed through the command via `CollectedAt`
- register mapping is configuration-driven
- invalid measurements are skipped and not persisted
- the default Modbus mapping uses holding registers `40100` and `40101`
- when `signed_values=true`, Modbus registers are decoded as signed 16-bit integers

## Configuration

The service loads configuration from environment variables.

Required variables:

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

Optional Influx write tuning:

```text
INFLUX_LOG_LEVEL
INFLUX_WRITE_MODE
INFLUX_BATCH_SIZE
INFLUX_FLUSH_INTERVAL
```

Optional tracing configuration:

```text
TRACING_ENABLED
TRACING_ENDPOINT
TRACING_INSECURE
TRACING_SAMPLE_RATIO
```

Example:

```bash
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

Influx write modes:

- `blocking`: write each point synchronously
- `batch`: group concurrent writes and flush them in batches before `Save(...)` returns

`INFLUX_LOG_LEVEL` maps directly to the InfluxDB Go client log level (`uint`).

When `INFLUX_WRITE_MODE=batch`, you can also set:

- `INFLUX_BATCH_SIZE`
- `INFLUX_FLUSH_INTERVAL`

Batch mode keeps repository success semantics honest: a `Save(...)` call returns only after the batch containing the point has been written successfully or failed. The tradeoff is added latency up to the configured flush interval.

Tracing is exported over OTLP/HTTP when `TRACING_ENABLED=true`.

## Run

```bash
go run ./cmd/integration-service
```

Or with the provided Make target:

```bash
make run-local
```

Endpoints:

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

Readiness behavior:

- `/healthz` reports that the process is running
- `/readyz` becomes healthy only after the service has completed at least one successful end-to-end collection and persistence cycle
- `/readyz` turns unhealthy again during shutdown or if successful collections go stale beyond the readiness window derived from the poll interval

`/metrics` is exposed with the official Prometheus Go client and includes both service telemetry counters and standard Go/process collectors.

Telemetry histograms exposed for percentile dashboards:

- `integration_service_telemetry_collection_duration_seconds`
- `integration_service_telemetry_source_read_duration_seconds`
- `integration_service_telemetry_persistence_duration_seconds`

Trace spans emitted by the worker pipeline:

- `telemetry.collect`
- `telemetry.source.read`
- `telemetry.persistence.save`

## Docker Compose

To run the Integration Service with InfluxDB and the `oitc/modbus-server:latest` Modbus server image:

```bash
docker compose up --build
```

Or:

```bash
make compose-up
```

Services started by the compose stack:

- `integration-service`
- `influxdb`
- `modbus-server`
- `prometheus`

The compose stack uses `oitc/modbus-server:latest` with a mounted config file at [docker/modbus/server_config.json](/Users/aliheydarabadii/Desktop/interview/Stellar/docker/modbus/server_config.json), including the holding register values for `40100` and `40101`.

The equivalent standalone Modbus server command is:

```bash
docker run --rm -p 5020:5020 \
  -v ./docker/modbus/server_config.json:/server_config.json:ro \
  oitc/modbus-server:latest -f /server_config.json
```

Exposed ports:

- `8080`: Integration Service HTTP endpoints
- `8086`: InfluxDB
- `5020`: Modbus server
- `9090`: Prometheus UI

Prometheus scrapes the Integration Service from `integration-service:8080/metrics`.

Example PromQL for p95 collection latency:

```promql
histogram_quantile(
  0.95,
  sum(rate(integration_service_telemetry_collection_duration_seconds_bucket[5m])) by (le)
)
```

## Testing

Run the full automated test suite with:

```bash
go test ./...
```

The test suite covers domain validation, application command orchestration, Modbus adapter behavior, InfluxDB point mapping/repository behavior, and worker loop behavior.

## Load Testing

This service does not expose a public telemetry ingestion endpoint. The worker drives collection internally, so the provided k6 scripts cover:

- HTTP surface load for `/healthz` and `/readyz`
- direct InfluxDB write load for repository throughput experiments

Run the HTTP load test:

```bash
k6 run loadtest/k6/service-http.js
```

Or:

```bash
make load-http
```

Run the Influx write load test:

```bash
INFLUX_URL=http://localhost:8086 \
INFLUX_ORG=local \
INFLUX_BUCKET=telemetry \
INFLUX_TOKEN=dev-token \
k6 run loadtest/k6/influx-write.js
```

Or:

```bash
make load-influx
```

Useful overrides:

- `BASE_URL`
- `VUS`
- `DURATION`
- `SLEEP_SECONDS`
- `SETPOINT`
- `ACTIVE_POWER`
