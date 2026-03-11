//go:build integration

package influxdb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"stellar/internal/measurements/app/query"
	"stellar/internal/measurements/testsupport"
)

type ReadModelIntegrationSuite struct {
	suite.Suite
}

func TestReadModelIntegrationSuite(t *testing.T) {
	suite.Run(t, new(ReadModelIntegrationSuite))
}

func (s *ReadModelIntegrationSuite) TestGetMeasurements() {
	influx := testsupport.StartInfluxDB(s.T())

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	seedFixtureMeasurements(s.T(), influx, base)

	readModel := NewReadModel(influx.Client, influx.Org, influx.Bucket, 10*time.Second, CircuitBreakerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	got, err := readModel.GetMeasurements(ctx, "asset-1", base, base.Add(4*time.Second+500*time.Millisecond))
	s.Require().NoError(err)

	want := []query.MeasurementPoint{
		{
			Timestamp:   base,
			Setpoint:    10,
			ActivePower: 9,
		},
		{
			Timestamp:   base.Add(time.Second),
			Setpoint:    20,
			ActivePower: 19,
		},
		{
			Timestamp:   base.Add(2 * time.Second),
			Setpoint:    31,
			ActivePower: 30,
		},
		{
			Timestamp:   base.Add(3 * time.Second),
			Setpoint:    40,
			ActivePower: 39,
		},
	}

	s.assertMeasurementSeries(got, want)
}

func (s *ReadModelIntegrationSuite) TestAppliesTimeRangeAndAssetFiltering() {
	influx := testsupport.StartInfluxDB(s.T())

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	seedFixtureMeasurements(s.T(), influx, base)

	readModel := NewReadModel(influx.Client, influx.Org, influx.Bucket, 10*time.Second, CircuitBreakerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	got, err := readModel.GetMeasurements(ctx, "asset-1", base.Add(time.Second), base.Add(2*time.Second+900*time.Millisecond))
	s.Require().NoError(err)

	want := []query.MeasurementPoint{
		{
			Timestamp:   base.Add(time.Second),
			Setpoint:    20,
			ActivePower: 19,
		},
		{
			Timestamp:   base.Add(2 * time.Second),
			Setpoint:    31,
			ActivePower: 30,
		},
	}

	s.assertMeasurementSeries(got, want)
}

func (s *ReadModelIntegrationSuite) assertMeasurementSeries(got, want []query.MeasurementPoint) {
	s.Require().Len(got, len(want))

	for i := range got {
		s.True(got[i].Timestamp.Equal(want[i].Timestamp), "point %d: expected timestamp %s, got %s", i, want[i].Timestamp, got[i].Timestamp)
		s.Equal(want[i].Setpoint, got[i].Setpoint, "point %d: expected setpoint", i)
		s.Equal(want[i].ActivePower, got[i].ActivePower, "point %d: expected active power", i)
		if i > 0 {
			s.False(got[i].Timestamp.Before(got[i-1].Timestamp), "points are not sorted ascending at index %d", i)
		}
	}
}

func seedFixtureMeasurements(t *testing.T, influx *testsupport.TestInflux, base time.Time) {
	t.Helper()

	testsupport.SeedMeasurements(t, influx.Client, influx.Org, influx.Bucket, []testsupport.MeasurementSeed{
		{
			AssetID:     "asset-1",
			Timestamp:   base,
			Setpoint:    testsupport.Float64Ptr(10),
			ActivePower: testsupport.Float64Ptr(9),
		},
		{
			AssetID:     "asset-1",
			Timestamp:   base.Add(time.Second + 100*time.Millisecond),
			Setpoint:    testsupport.Float64Ptr(20),
			ActivePower: testsupport.Float64Ptr(19),
		},
		{
			AssetID:   "asset-1",
			Timestamp: base.Add(2*time.Second + 100*time.Millisecond),
			Setpoint:  testsupport.Float64Ptr(30),
		},
		{
			AssetID:     "asset-1",
			Timestamp:   base.Add(2*time.Second + 100*time.Millisecond),
			ActivePower: testsupport.Float64Ptr(29),
		},
		{
			AssetID:     "asset-1",
			Timestamp:   base.Add(2*time.Second + 800*time.Millisecond),
			ActivePower: testsupport.Float64Ptr(30),
		},
		{
			AssetID:   "asset-1",
			Timestamp: base.Add(2*time.Second + 800*time.Millisecond),
			Setpoint:  testsupport.Float64Ptr(31),
		},
		{
			AssetID:   "asset-1",
			Timestamp: base.Add(3*time.Second + 100*time.Millisecond),
			Setpoint:  testsupport.Float64Ptr(40),
		},
		{
			AssetID:     "asset-1",
			Timestamp:   base.Add(3*time.Second + 100*time.Millisecond),
			ActivePower: testsupport.Float64Ptr(39),
		},
		{
			AssetID:   "asset-1",
			Timestamp: base.Add(3*time.Second + 700*time.Millisecond),
			Setpoint:  testsupport.Float64Ptr(41),
		},
		{
			AssetID:   "asset-1",
			Timestamp: base.Add(4*time.Second + 100*time.Millisecond),
			Setpoint:  testsupport.Float64Ptr(50),
		},
		{
			AssetID:     "asset-1",
			Timestamp:   base.Add(4*time.Second + 700*time.Millisecond),
			ActivePower: testsupport.Float64Ptr(49),
		},
		{
			AssetID:     "asset-2",
			Timestamp:   base.Add(time.Second + 200*time.Millisecond),
			Setpoint:    testsupport.Float64Ptr(999),
			ActivePower: testsupport.Float64Ptr(998),
		},
		{
			AssetID:     "asset-1",
			Timestamp:   base.Add(6 * time.Second),
			Setpoint:    testsupport.Float64Ptr(60),
			ActivePower: testsupport.Float64Ptr(59),
		},
	})
}
