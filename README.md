# Stellar Technical Assignment

This repository implements a small microservice-based telemetry system in Go for a single solar panel asset.

The configured asset id is `871689260010377213`.

## Overview

The system has three application services:

- `integration-service`: polls Modbus registers every second, validates the reading, and writes valid measurements to InfluxDB
- `measurement-service`: exposes an internal gRPC API for reading historical measurements from InfluxDB
- `api-gateway`: exposes the external REST API and applies the assignment's five-minute cache rule through Redis

Supporting infrastructure:

- `modbus-server`: local Modbus TCP simulator
- `influxdb`: time-series database for persisted measurements
- `redis`: cache for REST responses
- `prometheus`: optional metrics scraping for the integration service

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
