package influxdb

import (
	"context"

	"stellar/internal/telemetry/domain"
)

type MeasurementRepository struct {
	mapper *PointMapper
}

func NewMeasurementRepository(mapper *PointMapper) *MeasurementRepository {
	return &MeasurementRepository{
		mapper: mapper,
	}
}

func (r *MeasurementRepository) Save(_ context.Context, measurement domain.Measurement) error {
	_ = r.mapper.Map(measurement)
	// TODO: replace with real InfluxDB batch persistence.
	return nil
}
