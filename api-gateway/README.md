# API Gateway

The API Gateway is the external read-side entrypoint for historical asset measurements. It exposes a REST/JSON API, calls the internal Measurement Service over gRPC, and applies the assignment's five-minute freshness rule by caching full responses in Redis per exact request.

## Architecture

The service is organized around the measurements use case:

- `internal/measurements/application/get_measurements`: request DTOs, input and output ports, query validation, and the `GetMeasurements` use case
- `internal/measurements/domain`: read-side measurement entities returned by the gateway
- `internal/measurements/adapters/outbound/grpc`: gRPC client adapter for the Measurement Service
- `internal/measurements/adapters/outbound/redis`: Redis-backed cache adapter, cached reader decorator, and deterministic cache keys
- `internal/measurements/adapters/inbound/http`: REST API, middleware, health endpoints, and HTTP-specific request handling
- `internal/platform`: shared configuration, logging, and request context utilities
- `cmd/api-gateway`: bootstrap, dependency wiring, readiness composition, and graceful shutdown

This service is read-side only. It does not talk to Modbus, does not write to InfluxDB, and does not query InfluxDB directly.

## Package Structure

```text
cmd/
  api-gateway/
    main.go

internal/
  measurements/
    application/
      get_measurements/
        errors.go
        ports.go
        query.go
        usecase.go
    domain/
      measurement.go
    adapters/
      inbound/
        http/
          handler.go
          health.go
          middleware.go
      outbound/
        grpc/
          client.go
        redis/
          cache.go
          cached_reader.go
          key.go
  platform/
    config/
      config.go
    logging/
      cache_observer.go
      logger.go
    requestctx/
      requestctx.go
```

## Configuration

Environment variables:

- `MEASUREMENT_SERVICE_GRPC_ADDR`: Measurement Service gRPC address, required
- `HTTP_LISTEN_ADDR`: gateway HTTP listen address, default `:8080`
- `HEALTH_LISTEN_ADDR`: health HTTP listen address, default `:8081`; set it equal to `HTTP_LISTEN_ADDR` or empty to serve health on the main HTTP server
- `REDIS_ADDR`: Redis address, required
- `REDIS_USERNAME`: optional Redis username
- `REDIS_PASSWORD`: optional Redis password
- `REDIS_DB`: optional Redis DB, default `0`
- `CACHE_TTL`: cache TTL, default `5m`
- `REQUEST_TIMEOUT`: optional per-request timeout, default `10s`
- `READINESS_CHECK_TIMEOUT`: readiness probe timeout, default `2s`
- `HTTP_READ_HEADER_TIMEOUT`: HTTP server read-header timeout, default `5s`
- `HTTP_READ_TIMEOUT`: HTTP server read timeout, default `10s`
- `HTTP_WRITE_TIMEOUT`: HTTP server write timeout, default `15s`
- `HTTP_IDLE_TIMEOUT`: HTTP server idle timeout, default `60s`
- `HTTP_MAX_HEADER_BYTES`: maximum HTTP header size in bytes, default `1048576`

## Running

This gateway expects:

- the sibling `../measurement-service` module to expose the generated gRPC client package `stellar/api/proto`
- Redis to be available at `REDIS_ADDR`
- the Measurement Service to be reachable at `MEASUREMENT_SERVICE_GRPC_ADDR`

Example:

```bash
export MEASUREMENT_SERVICE_GRPC_ADDR=127.0.0.1:9090
export REDIS_ADDR=127.0.0.1:6379
go run ./cmd/api-gateway
```

The service starts:

- the REST API on `HTTP_LISTEN_ADDR`
- `/healthz` and `/readyz` on `HEALTH_LISTEN_ADDR` or on the main server when configured that way

Operational behavior:

- successful HTTP requests are logged with status, duration, request ID, correlation ID, and cache hit or miss
- `x-request-id` and `x-correlation-id` are propagated to the Measurement Service over gRPC
- `/readyz` actively checks both Redis and the Measurement Service before returning `200`
- the outbound Redis decorator applies the five-minute read cache and writes cache hit or miss status into request context for access logging

## API

Endpoint:

```text
GET /assets/{asset_id}/measurements?from=RFC3339&to=RFC3339
```

Example response:

```json
{
  "asset_id": "asset-1",
  "points": [
    {
      "timestamp": "2026-03-10T12:00:00Z",
      "setpoint": 100,
      "active_power": 55
    }
  ]
}
```

## Assumptions

- This service is the external REST gateway for measurement reads.
- It depends on the internal Measurement Service through unary gRPC.
- Redis stores the five-minute cache entries.
- Cache keys are based on the exact request identity: `asset_id`, `from`, and `to`.
- Redis read/write failures during request handling are logged and bypassed, but readiness still requires Redis availability because the five-minute freshness contract depends on it.

## Testing

Run the full test suite with:

```bash
go test ./...
```
