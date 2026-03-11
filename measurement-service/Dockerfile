FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src/measurement-service

COPY measurement-service/go.mod measurement-service/go.sum ./
RUN go mod download

COPY measurement-service ./

RUN target_os="${TARGETOS:-linux}" && \
	target_arch="${TARGETARCH:-$(go env GOARCH)}" && \
	CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build \
		-trimpath \
		-ldflags="-s -w" \
		-o /out/measurement-service \
		./cmd/measurement-service

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build /out/measurement-service /app/measurement-service

EXPOSE 9090 8080

ENTRYPOINT ["/app/measurement-service"]
