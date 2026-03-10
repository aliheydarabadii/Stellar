//go:build integration

package influxdb

import (
	"context"
	"testing"
	"time"

	"stellar/internal/measurements/app/query"
	"stellar/internal/measurements/testsupport"
)

func TestReadModelIntegrationGetMeasurements(t *testing.T) {
	influx := testsupport.StartInfluxDB(t)

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	seedFixtureMeasurements(t, influx, base)

	readModel := NewReadModel(influx.Client, influx.Org, influx.Bucket, 10*time.Second, CircuitBreakerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	got, err := readModel.GetMeasurements(ctx, "asset-1", base, base.Add(4*time.Second+500*time.Millisecond))
	if err != nil {
		t.Fatalf("get measurements: %v", err)
	}

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

	assertMeasurementSeries(t, got, want)
}

func TestReadModelIntegrationAppliesTimeRangeAndAssetFiltering(t *testing.T) {
	influx := testsupport.StartInfluxDB(t)

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	seedFixtureMeasurements(t, influx, base)

	readModel := NewReadModel(influx.Client, influx.Org, influx.Bucket, 10*time.Second, CircuitBreakerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	got, err := readModel.GetMeasurements(ctx, "asset-1", base.Add(time.Second), base.Add(2*time.Second+900*time.Millisecond))
	if err != nil {
		t.Fatalf("get measurements with range filter: %v", err)
	}

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

	assertMeasurementSeries(t, got, want)
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

func assertMeasurementSeries(t *testing.T, got, want []query.MeasurementPoint) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d points, got %d", len(want), len(got))
	}

	for i := range got {
		if !got[i].Timestamp.Equal(want[i].Timestamp) {
			t.Fatalf("point %d: expected timestamp %s, got %s", i, want[i].Timestamp, got[i].Timestamp)
		}
		if got[i].Setpoint != want[i].Setpoint {
			t.Fatalf("point %d: expected setpoint %v, got %v", i, want[i].Setpoint, got[i].Setpoint)
		}
		if got[i].ActivePower != want[i].ActivePower {
			t.Fatalf("point %d: expected active power %v, got %v", i, want[i].ActivePower, got[i].ActivePower)
		}
		if i > 0 && got[i].Timestamp.Before(got[i-1].Timestamp) {
			t.Fatalf("points are not sorted ascending at index %d", i)
		}
	}
}
