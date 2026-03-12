# API Gateway

The API Gateway is the external read-side entrypoint for historical asset measurements. It exposes a REST/JSON API, calls the internal Measurement Service over gRPC, and applies the assignment's five-minute freshness rule by caching full responses in Redis per exact request.

## Architecture

The service is organized around a single measurements read feature:

- `internal/measurements`: feature-level measurement types, the shared `MeasurementsReader` port, and reader-contract errors
- `internal/measurements/application`: query DTOs, application ports, validation, and the `GetMeasurementsHandler`
- `internal/measurements/adapters/inbound/http`: Gin-based REST handler, middleware, health endpoints, and HTTP-specific request handling
- `internal/measurements/adapters/outbound/grpc`: gRPC client adapter for the Measurement Service plus a circuit-breaker reader decorator
- `internal/measurements/adapters/outbound/redis`: Redis-backed cache adapter, cached reader decorator, and deterministic cache keys
- `internal/platform`: shared configuration, logging, and request context utilities
- `cmd/api-gateway`: bootstrap, dependency wiring, readiness composition, and graceful shutdown

## Package Structure

```text
cmd/
  api-gateway/
    main.go
    main_test.go

internal/
  measurements/
    adapters/
      inbound/
        http/
          handler.go
          handler_test.go
          health.go
          middleware.go
      outbound/
        grpc/
          circuit_breaker.go
          circuit_breaker_test.go
          client.go
          client_test.go
        redis/
          cache.go
          cache_test.go
          cached_reader.go
          cached_reader_test.go
          key.go
    application/
      errors.go
      ports.go
      query.go
      query_handler.go
      query_handler_test.go
    errors.go
    measurement.go
    mocks/
      MeasurementsReader.go
    ports.go
  platform/
    config/
      config.go
      config_test.go
    logging/
      cache_observer.go
      logger.go
      logger_test.go
    requestctx/
      requestctx.go
```

## Configuration

Environment variables:

- `LOG_LEVEL`: optional structured log level, default `INFO`
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
- the Redis cached reader applies the five-minute freshness policy; cache hits stay in Redis, and cache misses flow through a circuit-breaker-protected gRPC reader
- `/readyz` actively checks both Redis and the Measurement Service before returning `200`
- the outbound Redis decorator writes cache hit, miss, bypass, or not-applicable status into request context for access logging

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
