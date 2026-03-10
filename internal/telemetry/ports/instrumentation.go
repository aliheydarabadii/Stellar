package ports

import (
	"context"
	"time"

	"stellar/internal/telemetry/app/command"
	"stellar/internal/telemetry/domain"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func InstrumentTelemetrySource(next command.TelemetrySource, metrics *Metrics, tracer trace.Tracer) command.TelemetrySource {
	if next == nil {
		return next
	}

	if metrics == nil {
		metrics = NewMetrics()
	}

	if tracer == nil {
		tracer = otel.Tracer("stellar/internal/telemetry/ports")
	}

	return instrumentedTelemetrySource{
		next:    next,
		metrics: metrics,
		tracer:  tracer,
	}
}

func InstrumentMeasurementRepository(next command.MeasurementRepository, metrics *Metrics, tracer trace.Tracer) command.MeasurementRepository {
	if next == nil {
		return next
	}

	if metrics == nil {
		metrics = NewMetrics()
	}

	if tracer == nil {
		tracer = otel.Tracer("stellar/internal/telemetry/ports")
	}

	return instrumentedMeasurementRepository{
		next:    next,
		metrics: metrics,
		tracer:  tracer,
	}
}

type instrumentedTelemetrySource struct {
	next    command.TelemetrySource
	metrics *Metrics
	tracer  trace.Tracer
}

func (s instrumentedTelemetrySource) Read(ctx context.Context) (command.TelemetryReading, error) {
	ctx, span := s.tracer.Start(ctx, "telemetry.source.read", trace.WithSpanKind(trace.SpanKindClient))
	startedAt := time.Now()
	reading, err := s.next.Read(ctx)
	s.metrics.ObserveSourceReadDuration(time.Since(startedAt))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return reading, err
	}

	span.SetStatus(codes.Ok, "")
	span.End()

	return reading, err
}

type instrumentedMeasurementRepository struct {
	next    command.MeasurementRepository
	metrics *Metrics
	tracer  trace.Tracer
}

func (r instrumentedMeasurementRepository) Save(ctx context.Context, measurement domain.Measurement) error {
	ctx, span := r.tracer.Start(
		ctx,
		"telemetry.persistence.save",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("asset.id", measurement.AssetID.String())),
	)
	startedAt := time.Now()
	err := r.next.Save(ctx, measurement)
	r.metrics.ObservePersistenceDuration(time.Since(startedAt))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return err
	}

	span.SetStatus(codes.Ok, "")
	span.End()

	return err
}
