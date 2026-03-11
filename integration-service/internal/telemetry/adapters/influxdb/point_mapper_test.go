package influxdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"stellar/internal/telemetry/domain"
)

type PointMapperTestSuite struct {
	suite.Suite
	collectedAt time.Time
	measurement domain.Measurement
}

func TestPointMapperTestSuite(t *testing.T) {
	suite.Run(t, new(PointMapperTestSuite))
}

func (s *PointMapperTestSuite) SetupTest() {
	s.collectedAt = time.Date(2026, time.March, 10, 9, 30, 0, 0, time.UTC)

	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 55, s.collectedAt)
	s.Require().NoError(err)
	s.measurement = measurement
}

func (s *PointMapperTestSuite) TestPointMapperMap() {
	mapper := NewPointMapper()
	point := mapper.Map(s.measurement)

	s.Equal(assetMeasurementsName, point.Name)
	s.Equal(domain.DefaultAssetID.String(), point.Tags.AssetID)
	s.Empty(point.Tags.AssetType)
	s.Equal(float64(100), point.Fields.Setpoint)
	s.Equal(float64(55), point.Fields.ActivePower)
	s.True(point.Timestamp.Equal(s.collectedAt))
}

func (s *PointMapperTestSuite) TestPointMapperMapWithAssetType() {
	mapper := NewPointMapperWithAssetType("solar_panel")
	point := mapper.Map(s.measurement)

	s.Equal("solar_panel", point.Tags.AssetType)
}
