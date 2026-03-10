package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"stellar/internal/measurements/app/query"
)

func TestNewRejectsNilReadModel(t *testing.T) {
	t.Parallel()

	_, err := New(nil)
	if !errors.Is(err, query.ErrReadModelUnavailable) {
		t.Fatalf("expected ErrReadModelUnavailable, got %v", err)
	}
}

func TestNewBuildsApplicationWithValidReadModel(t *testing.T) {
	t.Parallel()

	application, err := New(appReadModelStub{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	series, err := application.Queries.GetMeasurements.Handle(context.Background(), query.GetMeasurements{
		AssetID: "asset-1",
		From:    time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 3, 10, 12, 0, 1, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected handler to be wired, got %v", err)
	}
	if series.AssetID != "asset-1" {
		t.Fatalf("expected asset-1, got %q", series.AssetID)
	}
}

type appReadModelStub struct{}

func (appReadModelStub) GetMeasurements(context.Context, string, time.Time, time.Time) ([]query.MeasurementPoint, error) {
	return nil, nil
}
