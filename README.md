# Stellar Technical Assignment

This repository implements a small microservice-based telemetry system in Go for a single solar panel asset.

The configured asset id is `871689260010377213`.

## Overview

The assignment requires three application services:

- `integration-service`: polls Modbus registers every second, validates the reading, and writes valid measurements to InfluxDB
- `measurement-service`: exposes an internal gRPC API for reading historical measurements from InfluxDB
- `api-gateway`: exposes the external REST API and applies the assignment's five-minute cache rule through Redis

Supporting infrastructure:

- `modbus-server`: local Modbus TCP simulator
- `influxdb`: time-series database for persisted measurements
- `redis`: cache for REST responses
- `prometheus`: optional metrics scraping for the integration service

## Requirement Coverage

- `integration-service` polls Modbus holding registers `40100` and `40101` every second, maps them to `setpoint` and `active_power`, validates the readings, and writes valid measurements to InfluxDB for asset `871689260010377213`
- `measurement-service` exposes the required internal gRPC API and returns one-second resolution measurements for an asset and time window
- `api-gateway` exposes the required external REST API and applies the five-minute freshness rule for external clients
- validation rules from the assignment are enforced before persistence:
  - negative values are rejected
  - `active_power > setpoint` is rejected
- the repository includes tests, structured logging, Docker Compose setup, and service-level documentation

## Runtime Flow

1. The Modbus server exposes holding registers `40100` and `40101`.
2. The integration service reads them every second.
3. The values are mapped to `setpoint` and `active_power`.
4. Invalid measurements are rejected:
   - negative values are skipped
   - `active_power > setpoint` is skipped
5. Valid measurements are written to InfluxDB.
6. The measurement service reads one-second resolution points from InfluxDB over gRPC.
7. The API gateway serves REST responses and caches each exact request for five minutes.

## Architecture

```text
Modbus Server
    |
    v
Integration Service ---> InfluxDB
                              |
                              v
                    Measurement Service (gRPC)
                              |
                              v
                     API Gateway (REST + Redis cache)
```

## Implementation Notes

- The service split follows the assignment directly: ingestion stays in `integration-service`, internal querying stays in `measurement-service`, and the external contract stays in `api-gateway`
- The five-minute freshness rule is enforced in the API Gateway cache because that requirement applies to external clients, not to the internal gRPC API. This keeps the measurement service reusable and avoids coupling external caching policy to the internal read model
- InfluxDB is used as both the persistence layer and read model in this solution because the assignment already requires it for writes, and the query pattern is naturally time-series by asset and time window. That keeps the design smaller without adding an extra projection store

## Readiness Semantics

- `integration-service`
  - `/healthz` means the process is alive
  - `/readyz` becomes healthy only after at least one successful end-to-end poll and write cycle, and becomes unhealthy again if successful collection goes stale
- `measurement-service`
  - `/healthz` means the process is alive
  - `/readyz` means the service has started and is ready to accept traffic on its gRPC and health endpoints
- `api-gateway`
  - `/healthz` means the process is alive
  - `/readyz` checks that the gateway has started, Redis is reachable, and the measurement service is reachable

## Operational Hardening

- request and correlation IDs are accepted at the gateway, returned on HTTP responses, and propagated to the measurement service over gRPC
- the API Gateway emits structured access logs for successful requests, including status, duration, request ID, correlation ID, and cache hit or miss
- the gateway uses defensive HTTP server settings such as read, write, and idle timeouts plus a bounded maximum header size
- readiness checks are used to avoid routing traffic to services that are started but not actually usable

## Known Limitations

- authentication and authorization are intentionally omitted, per the assignment
- TLS is not configured for REST, gRPC, Redis, or InfluxDB in this local setup
- secrets are provided through local environment variables and Docker Compose rather than an infrastructure-managed secret store
- rate limiting is not implemented; if added later, it should be documented clearly whether it is per-instance or distributed
- Redis and InfluxDB are treated as single local dependencies in this assignment-oriented setup rather than highly available production services

## Run The Whole Stack

From the repository root:

```bash
docker compose up --build
```

To stop it:

```bash
docker compose down
```

## Main Ports

- `8080`: API Gateway REST API
- `8081`: Integration Service HTTP endpoints
- `8082`: Measurement Service health endpoints
- `9091`: Measurement Service gRPC
- `8086`: InfluxDB
- `6379`: Redis
- `5020`: Modbus TCP server
- `9090`: Prometheus

## Quick Test

After the stack has been running for a few seconds, call the REST API with a recent UTC time window. Replace the timestamps with values around the current UTC time when you run it.

```bash
curl -s "http://localhost:8080/assets/871689260010377213/measurements?from=<RFC3339_FROM>&to=<RFC3339_TO>"
```

Example:

```bash
curl -s "http://localhost:8080/assets/871689260010377213/measurements?from=2026-03-11T21:00:00Z&to=2026-03-11T21:05:00Z"
```

Basic health checks:

```bash
curl -i http://localhost:8080/healthz
curl -i http://localhost:8081/readyz
curl -i http://localhost:8082/readyz
```

## Repository Layout

```text
.
├── api-gateway/
├── integration-service/
├── measurement-service/
└── docker-compose.yml
```

## More Detail

- [integration-service/README.md](integration-service/README.md)
- [measurement-service/README.md](measurement-service/README.md)
- [api-gateway/README.md](api-gateway/README.md)
