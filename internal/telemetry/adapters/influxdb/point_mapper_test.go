package influxdb

import (
	"testing"
	"time"

	"stellar/internal/telemetry/domain"
)

func TestPointMapperMap(t *testing.T) {
	t.Parallel()

	collectedAt := time.Date(2026, time.March, 10, 9, 30, 0, 0, time.UTC)
	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 55, collectedAt)
	if err != nil {
		t.Fatalf("expected valid measurement, got %v", err)
	}

	mapper := NewPointMapper()
	point := mapper.Map(measurement)

	if point.Name != assetMeasurementsName {
		t.Fatalf("expected point name %q, got %q", assetMeasurementsName, point.Name)
	}

	if point.Tags.AssetID != domain.DefaultAssetID.String() {
		t.Fatalf("expected asset_id tag %q, got %q", domain.DefaultAssetID, point.Tags.AssetID)
	}

	if AssetType := point.Tags.AssetType; AssetType != "" {
		t.Fatal("did not expect asset_type tag for default mapper")
	}

	if point.Fields.Setpoint != 100 {
		t.Fatalf("expected setpoint field %v, got %v", 100, point.Fields.Setpoint)
	}

	if point.Fields.ActivePower != 55 {
		t.Fatalf("expected active_power field %v, got %v", 55, point.Fields.ActivePower)
	}

	if !point.Timestamp.Equal(collectedAt) {
		t.Fatalf("expected timestamp %v, got %v", collectedAt, point.Timestamp)
	}
}

func TestPointMapperMapWithAssetType(t *testing.T) {
	t.Parallel()

	collectedAt := time.Date(2026, time.March, 10, 9, 30, 0, 0, time.UTC)
	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 55, collectedAt)
	if err != nil {
		t.Fatalf("expected valid measurement, got %v", err)
	}

	mapper := NewPointMapperWithAssetType("solar_panel")
	point := mapper.Map(measurement)

	if point.Tags.AssetType != "solar_panel" {
		t.Fatalf("expected asset_type tag %q, got %q", "solar_panel", point.Tags.AssetType)
	}
}
