# API Gateway

The API Gateway is the external read-side entrypoint for historical asset measurements. It exposes a REST/JSON API, calls the internal Measurement Service over gRPC, and applies the assignment's five-minute freshness rule by caching full responses in Redis per exact request.

## Architecture

The service keeps orchestration in a thin application layer:

- `internal/gateway/app`: application wiring and the `GetMeasurements` query handler
- `internal/gateway/adapters/grpc`: gRPC client adapter for the Measurement Service
- `internal/gateway/adapters/cache`: Redis-backed cache adapter and deterministic cache keys
- `internal/gateway/ports`: HTTP API and health endpoints
- `cmd/api-gateway`: bootstrap, config, wiring, logging, and graceful shutdown

This service is read-side only. It does not talk to Modbus, does not write to InfluxDB, and does not query InfluxDB directly.

## Package Structure

```text
cmd/
  api-gateway/
    main.go

internal/
  gateway/
    app/
      app.go
      query/
        get_measurements.go
        types.go

    ports/
      http.go
      health.go

    adapters/
      grpc/
        measurements_client.go
      cache/
        redis_cache.go
        key.go
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
- Cache failures degrade gracefully: Redis read/write failures are logged and bypassed, but successful fresh responses still return to the caller.

## Testing

Run the full test suite with:

```bash
go test ./...
```
