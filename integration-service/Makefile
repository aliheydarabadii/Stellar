ASSET_ID ?= 871689260010377213
ASSET_TYPE ?= solar_panel
MODBUS_HOST ?= 127.0.0.1
MODBUS_PORT ?= 5020
MODBUS_UNIT_ID ?= 1
MODBUS_REGISTER_TYPE ?= holding
MODBUS_SETPOINT_ADDRESS ?= 40100
MODBUS_ACTIVE_POWER_ADDRESS ?= 40101
MODBUS_SIGNED_VALUES ?= true
POLL_INTERVAL ?= 1s
INFLUX_URL ?= http://localhost:8086
INFLUX_TOKEN ?= dev-token
INFLUX_ORG ?= local
INFLUX_BUCKET ?= telemetry
INFLUX_LOG_LEVEL ?= 0
INFLUX_WRITE_MODE ?= blocking
INFLUX_BATCH_SIZE ?=
INFLUX_FLUSH_INTERVAL ?=
TRACING_ENABLED ?= false
TRACING_ENDPOINT ?=
TRACING_INSECURE ?= true
TRACING_SAMPLE_RATIO ?= 1.0
HTTP_PORT ?= 8080

BASE_URL ?= http://localhost:$(HTTP_PORT)
VUS ?= 20
DURATION ?= 30s
SLEEP_SECONDS ?= 0.2
SETPOINT ?= 100
ACTIVE_POWER ?= 55

.PHONY: help test dev-up dev-down compose-up compose-down compose-logs run-local load-http load-influx

help:
	@echo "Available targets:"
	@echo "  make test          - run the Go test suite"
	@echo "  make dev-up        - start local dependencies (InfluxDB + Modbus server + Prometheus)"
	@echo "  make dev-down      - stop local dependencies"
	@echo "  make compose-up    - start the full compose stack"
	@echo "  make compose-down  - stop the full compose stack"
	@echo "  make compose-logs  - tail compose logs"
	@echo "  make run-local     - run the Integration Service locally with env defaults"
	@echo "  make load-http     - run the k6 health/readiness load test"
	@echo "  make load-influx   - run the k6 direct Influx write load test"

test:
	go test ./...

dev-up:
	docker compose up -d influxdb modbus-server prometheus

dev-down:
	docker compose down

compose-up:
	docker compose up --build

compose-down:
	docker compose down

compose-logs:
	docker compose logs -f

run-local:
	ASSET_ID=$(ASSET_ID) \
	ASSET_TYPE=$(ASSET_TYPE) \
	MODBUS_HOST=$(MODBUS_HOST) \
	MODBUS_PORT=$(MODBUS_PORT) \
	MODBUS_UNIT_ID=$(MODBUS_UNIT_ID) \
	MODBUS_REGISTER_TYPE=$(MODBUS_REGISTER_TYPE) \
	MODBUS_SETPOINT_ADDRESS=$(MODBUS_SETPOINT_ADDRESS) \
	MODBUS_ACTIVE_POWER_ADDRESS=$(MODBUS_ACTIVE_POWER_ADDRESS) \
	MODBUS_SIGNED_VALUES=$(MODBUS_SIGNED_VALUES) \
	POLL_INTERVAL=$(POLL_INTERVAL) \
	INFLUX_URL=$(INFLUX_URL) \
	INFLUX_TOKEN=$(INFLUX_TOKEN) \
	INFLUX_ORG=$(INFLUX_ORG) \
	INFLUX_BUCKET=$(INFLUX_BUCKET) \
	INFLUX_LOG_LEVEL=$(INFLUX_LOG_LEVEL) \
	INFLUX_WRITE_MODE=$(INFLUX_WRITE_MODE) \
	INFLUX_BATCH_SIZE=$(INFLUX_BATCH_SIZE) \
	INFLUX_FLUSH_INTERVAL=$(INFLUX_FLUSH_INTERVAL) \
	TRACING_ENABLED=$(TRACING_ENABLED) \
	TRACING_ENDPOINT=$(TRACING_ENDPOINT) \
	TRACING_INSECURE=$(TRACING_INSECURE) \
	TRACING_SAMPLE_RATIO=$(TRACING_SAMPLE_RATIO) \
	HTTP_PORT=$(HTTP_PORT) \
	go run ./cmd/integration-service

load-http:
	BASE_URL=$(BASE_URL) \
	VUS=$(VUS) \
	DURATION=$(DURATION) \
	SLEEP_SECONDS=$(SLEEP_SECONDS) \
	k6 run loadtest/k6/service-http.js

load-influx:
	INFLUX_URL=$(INFLUX_URL) \
	INFLUX_ORG=$(INFLUX_ORG) \
	INFLUX_BUCKET=$(INFLUX_BUCKET) \
	INFLUX_TOKEN=$(INFLUX_TOKEN) \
	VUS=$(VUS) \
	DURATION=$(DURATION) \
	SETPOINT=$(SETPOINT) \
	ACTIVE_POWER=$(ACTIVE_POWER) \
	k6 run loadtest/k6/influx-write.js
