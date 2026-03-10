package influxdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"stellar/internal/measurements/app/query"
)

func TestReadModelGetMeasurementsMapsInfluxRows(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 100_000_000, time.UTC)
	model := &ReadModel{
		bucket: "measurements",
		query: fakeQueryExecutor{
			records: []influxRecord{
				{Time: base, Field: "setpoint", Value: 10.0},
				{Time: base, Field: "active_power", Value: 9.5},
			},
		},
	}

	got, err := model.GetMeasurements(context.Background(), "asset-1", base.Add(-time.Minute), base.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := []query.MeasurementPoint{
		{
			Timestamp:   base.Truncate(time.Second),
			Setpoint:    10,
			ActivePower: 9.5,
		},
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d points, got %d", len(want), len(got))
	}

	if got[0] != want[0] {
		t.Fatalf("expected %+v, got %+v", want[0], got[0])
	}
}

func TestReadModelGetMeasurementsHandlesEmptyResult(t *testing.T) {
	t.Parallel()

	model := &ReadModel{
		bucket: "measurements",
		query:  fakeQueryExecutor{},
	}

	got, err := model.GetMeasurements(context.Background(), "asset-1", time.Now().Add(-time.Minute), time.Now())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected no points, got %d", len(got))
	}
}

func TestReadModelGetMeasurementsReturnsPointsOrderedByTimestamp(t *testing.T) {
	t.Parallel()

	first := time.Date(2026, 3, 10, 12, 0, 0, 500_000_000, time.UTC)
	second := first.Add(time.Second)
	model := &ReadModel{
		bucket: "measurements",
		query: fakeQueryExecutor{
			records: []influxRecord{
				{Time: second, Field: "setpoint", Value: 12.0},
				{Time: second, Field: "active_power", Value: 11.5},
				{Time: first, Field: "setpoint", Value: 10.0},
				{Time: first, Field: "active_power", Value: 9.5},
			},
		},
	}

	got, err := model.GetMeasurements(context.Background(), "asset-1", first.Add(-time.Minute), second.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 points, got %d", len(got))
	}

	if !got[0].Timestamp.Before(got[1].Timestamp) {
		t.Fatalf("expected ascending timestamps, got %v then %v", got[0].Timestamp, got[1].Timestamp)
	}
}

func TestReadModelGetMeasurementsLatestPointWinsWithinSecond(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 100_000_000, time.UTC)
	later := base.Add(700 * time.Millisecond)
	model := &ReadModel{
		bucket: "measurements",
		query: fakeQueryExecutor{
			records: []influxRecord{
				{Time: base, Field: "setpoint", Value: 10.0},
				{Time: base, Field: "active_power", Value: 9.0},
				{Time: later, Field: "setpoint", Value: 13.0},
				{Time: later, Field: "active_power", Value: 12.5},
			},
		},
	}

	got, err := model.GetMeasurements(context.Background(), "asset-1", base.Add(-time.Minute), base.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 point, got %d", len(got))
	}

	if got[0].Setpoint != 13.0 || got[0].ActivePower != 12.5 {
		t.Fatalf("expected latest values, got %+v", got[0])
	}
}

func TestReadModelGetMeasurementsReturnsExecutorError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("query failed")
	model := &ReadModel{
		bucket: "measurements",
		query:  fakeQueryExecutor{err: wantErr},
	}

	_, err := model.GetMeasurements(context.Background(), "asset-1", time.Now().Add(-time.Minute), time.Now())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}

type fakeQueryExecutor struct {
	records []influxRecord
	err     error
}

func (f fakeQueryExecutor) Query(_ context.Context, _ string) (recordIterator, error) {
	if f.err != nil {
		return nil, f.err
	}

	return &sliceRecordIterator{records: f.records}, nil
}

type sliceRecordIterator struct {
	records []influxRecord
	index   int
}

func (i *sliceRecordIterator) Next() bool {
	if i.index >= len(i.records) {
		return false
	}

	i.index++
	return true
}

func (i *sliceRecordIterator) Record() influxRecord {
	return i.records[i.index-1]
}

func (i *sliceRecordIterator) Err() error {
	return nil
}
