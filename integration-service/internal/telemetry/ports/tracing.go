package ports

import (
	"context"
	"fmt"
	"net/url"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type TracingConfig struct {
	Enabled     bool
	Endpoint    string
	Insecure    bool
	SampleRatio float64
}

func SetupTracing(ctx context.Context, serviceName string, config TracingConfig) (func(context.Context) error, error) {
	if !config.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	if config.Endpoint == "" {
		return nil, fmt.Errorf("tracing config: endpoint must not be empty when tracing is enabled")
	}

	if _, err := url.ParseRequestURI(config.Endpoint); err != nil {
		return nil, fmt.Errorf("tracing config: parse endpoint: %w", err)
	}

	if config.SampleRatio < 0 || config.SampleRatio > 1 {
		return nil, fmt.Errorf("tracing config: sample ratio must be between 0 and 1")
	}

	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(config.Endpoint),
	}
	if config.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(config.SampleRatio))),
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", serviceName),
		)),
	)

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider.Shutdown, nil
}
