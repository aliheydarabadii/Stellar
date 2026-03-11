# Measurement Service

The Measurement Service is the query-side microservice for historical asset measurements in the backend system. It exposes an internal gRPC API, reads measurement data from InfluxDB, and returns `setpoint` plus `active_power` as a one-second-resolution time series for a requested asset and time window.

## Architecture

The service follows a pragmatic DDD-lite, CQRS, and Clean Architecture layout inspired by "Go with the Domain":

- `internal/measurements/app`: thin application layer and query handlers
- `internal/measurements/ports`: delivery adapters for gRPC and health endpoints
- `internal/measurements/adapters/influxdb`: InfluxDB-backed read model adapter
- `api/proto`: gRPC contract
- `cmd/measurement-service`: bootstrap and wiring

This service is query-side only. It does not perform writes, does not talk to Modbus, and does not implement the API Gateway's external five-minute cache policy.

## Package Structure

```text
api/
  proto/
    measurements.proto

cmd/
  measurement-service/
    main.go

internal/
  measurements/
    app/
      app.go
      query/
        get_measurements.go
        types.go

    ports/
      grpc.go
      health.go

    adapters/
      influxdb/
        read_model.go
        mapper.go
```

Generated protobuf stubs live alongside the proto source in `api/proto`.

## Configuration

The service reads configuration from environment variables:

- `INFLUX_URL`: InfluxDB base URL, required
- `INFLUX_ORG`: InfluxDB organization, required
- `INFLUX_BUCKET`: InfluxDB bucket, required
- `INFLUX_TOKEN`: InfluxDB token, required
- `GRPC_LISTEN_ADDR`: gRPC listen address, default `:9090`
- `GRPC_CONNECTION_TIMEOUT`: gRPC connection timeout, default `5s`
- `GRPC_MAX_RECV_MSG_SIZE_BYTES`: gRPC max receive message size, default `4194304`
- `GRPC_MAX_SEND_MSG_SIZE_BYTES`: gRPC max send message size, default `4194304`
- `GRPC_KEEPALIVE_TIME`: gRPC keepalive ping interval, default `2m`
- `GRPC_KEEPALIVE_TIMEOUT`: gRPC keepalive timeout, default `20s`
- `GRPC_KEEPALIVE_MIN_TIME`: minimum client keepalive interval allowed by the server, default `1m`
- `HEALTH_LISTEN_ADDR`: health HTTP listen address, default `:8080`
- `MAX_QUERY_RANGE`: optional maximum request window, default `15m`
- `QUERY_TIMEOUT`: optional positive query timeout duration, default `10s`
- `INFLUX_CIRCUIT_BREAKER_FAILURE_THRESHOLD`: optional consecutive failure threshold before opening the breaker, default `5`
- `INFLUX_CIRCUIT_BREAKER_OPEN_TIMEOUT`: optional time the breaker stays open before half-open probe requests are allowed, default `30s`
- `INFLUX_CIRCUIT_BREAKER_HALF_OPEN_MAX_REQUESTS`: optional max concurrent probe requests in half-open state, default `1`

## Running

Common commands:

```bash
make proto
make run-measurements
make test
make test-integration
```

1. Generate protobuf code when the contract changes:

```bash
PATH="$(go env GOPATH)/bin:$PATH" \
protoc \
  --go_out=paths=source_relative:. \
  --go-grpc_out=paths=source_relative:. \
  api/proto/measurements.proto
```

2. Set the required environment variables and run the service:

```bash
go run ./cmd/measurement-service
```

The service starts:

- a gRPC server on `GRPC_LISTEN_ADDR`
- an HTTP health server with `/healthz` and `/readyz` on `HEALTH_LISTEN_ADDR`

The gRPC server includes panic recovery and returns `Internal` instead of crashing the process if a handler panics.
It also accepts `x-request-id` or `x-correlation-id` gRPC metadata, propagates the request ID through the service context, echoes it back as `x-request-id`, and includes it in request logs.

## Assumptions

- This service is query-side only in CQRS terms.
- It exposes an internal unary gRPC API.
- InfluxDB is the read model and stores records in `asset_measurements`.
- Measurements are filtered by `asset_id`.
- The response time series is one-second resolution.
- Requests larger than the configured `MAX_QUERY_RANGE` are rejected. The default max range is `15m`.
- If multiple writes exist within the same second, the latest exact timestamp in that second that has both `setpoint` and `active_power` wins.
- A response point is emitted only when both `setpoint` and `active_power` exist at the same exact timestamp within that second.
- Missing seconds are not interpolated.
- Repeated InfluxDB failures open a circuit breaker to fail fast until the cool-down window expires.
- The external five-minute cache behavior belongs to the API Gateway, not this service.

## Testing

Run unit tests with:

```bash
go test ./...
```

Run the Docker-backed integration tests against a real InfluxDB 2.x instance with:

```bash
go test -tags=integration ./internal/measurements/adapters/influxdb ./internal/measurements/ports
```

Integration tests require Docker because they start a real InfluxDB 2.x container.
