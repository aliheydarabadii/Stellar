FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY measurement-service/go.mod measurement-service/go.sum ./measurement_service/
COPY api-gateway/go.mod api-gateway/go.sum ./api-gateway/

WORKDIR /src/api-gateway
RUN go mod download

COPY measurement-service /src/measurement_service
COPY api-gateway /src/api-gateway

RUN target_os="${TARGETOS:-linux}" && \
	target_arch="${TARGETARCH:-$(go env GOARCH)}" && \
	CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build \
		-trimpath \
		-ldflags="-s -w" \
		-o /out/api-gateway \
		./cmd/api-gateway

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build /out/api-gateway /app/api-gateway

EXPOSE 8080 8081

ENTRYPOINT ["/app/api-gateway"]
