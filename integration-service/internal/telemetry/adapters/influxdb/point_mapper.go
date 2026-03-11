package influxdb

import (
	"time"

	"stellar/internal/telemetry/domain"
)

const assetMeasurementsName = "asset_measurements"

type Point struct {
	Name      string
	Tags      PointTags
	Fields    PointFields
	Timestamp time.Time
}

type PointTags struct {
	AssetID   string
	AssetType string
}

type PointFields struct {
	Setpoint    float64
	ActivePower float64
}

type PointMapper struct {
	assetType string
}

func NewPointMapper() *PointMapper {
	return &PointMapper{}
}

func NewPointMapperWithAssetType(assetType string) *PointMapper {
	return &PointMapper{assetType: assetType}
}

func (m *PointMapper) Map(measurement domain.Measurement) Point {

	tags := PointTags{AssetID: measurement.AssetID.String()}

	if m.assetType != "" {
		tags.AssetType = m.assetType
	}

	return Point{
		Name: assetMeasurementsName,
		Tags: tags,
		Fields: PointFields{
			Setpoint:    measurement.Setpoint,
			ActivePower: measurement.ActivePower,
		},
		Timestamp: measurement.CollectedAt,
	}
}
