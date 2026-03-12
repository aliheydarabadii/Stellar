GO ?= go
PROTOC ?= protoc
GOCACHE ?= $(CURDIR)/.cache/go-build

.PHONY: test test-integration proto run-measurements

test:
	GOCACHE=$(GOCACHE) $(GO) test ./...

test-integration:
	GOCACHE=$(GOCACHE) $(GO) test -tags=integration ./internal/measurements/adapters/outbound/influxdb ./internal/measurements/adapters/inbound/grpc

proto:
	PATH="$(shell $(GO) env GOPATH)/bin:$(PATH)" $(PROTOC) \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		api/proto/measurements.proto

run-measurements:
	GOCACHE=$(GOCACHE) $(GO) run ./cmd/measurement-service
