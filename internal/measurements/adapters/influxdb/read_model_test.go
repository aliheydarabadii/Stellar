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

func TestReadModelGetMeasurementsSelectsLatestCompletePointWithinSecond(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	testCases := []struct {
		name    string
		records []influxRecord
		want    []query.MeasurementPoint
	}{
		{
			name: "later complete point wins",
			records: []influxRecord{
				{Time: base.Add(100 * time.Millisecond), Field: "setpoint", Value: 10.0},
				{Time: base.Add(100 * time.Millisecond), Field: "active_power", Value: 9.0},
				{Time: base.Add(700 * time.Millisecond), Field: "setpoint", Value: 13.0},
				{Time: base.Add(700 * time.Millisecond), Field: "active_power", Value: 12.5},
			},
			want: []query.MeasurementPoint{
				{
					Timestamp:   base,
					Setpoint:    13.0,
					ActivePower: 12.5,
				},
			},
		},
		{
			name: "earlier complete point wins when later timestamp is incomplete",
			records: []influxRecord{
				{Time: base.Add(100 * time.Millisecond), Field: "setpoint", Value: 10.0},
				{Time: base.Add(100 * time.Millisecond), Field: "active_power", Value: 9.0},
				{Time: base.Add(700 * time.Millisecond), Field: "setpoint", Value: 13.0},
			},
			want: []query.MeasurementPoint{
				{
					Timestamp:   base,
					Setpoint:    10.0,
					ActivePower: 9.0,
				},
			},
		},
		{
			name: "interleaved timestamps are not mixed together",
			records: []influxRecord{
				{Time: base.Add(100 * time.Millisecond), Field: "setpoint", Value: 10.0},
				{Time: base.Add(400 * time.Millisecond), Field: "active_power", Value: 12.5},
				{Time: base.Add(700 * time.Millisecond), Field: "setpoint", Value: 14.0},
				{Time: base.Add(100 * time.Millisecond), Field: "active_power", Value: 9.0},
			},
			want: []query.MeasurementPoint{
				{
					Timestamp:   base,
					Setpoint:    10.0,
					ActivePower: 9.0,
				},
			},
		},
		{
			name: "second is dropped when no timestamp has both fields",
			records: []influxRecord{
				{Time: base.Add(100 * time.Millisecond), Field: "setpoint", Value: 10.0},
				{Time: base.Add(700 * time.Millisecond), Field: "active_power", Value: 12.5},
			},
			want: nil,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			model := &ReadModel{
				bucket: "measurements",
				query: fakeQueryExecutor{
					records: tc.records,
				},
			}

			got, err := model.GetMeasurements(context.Background(), "asset-1", base.Add(-time.Minute), base.Add(time.Minute))
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if len(got) != len(tc.want) {
				t.Fatalf("expected %d points, got %d", len(tc.want), len(got))
			}

			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("expected %+v, got %+v", tc.want[i], got[i])
				}
			}
		})
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

func TestReadModelGetMeasurementsOpensCircuitAfterThreshold(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	executor := &countingQueryExecutor{err: errors.New("influx unavailable")}
	model := &ReadModel{
		bucket: "measurements",
		query:  executor,
		breaker: newCircuitBreaker(CircuitBreakerConfig{
			FailureThreshold:    2,
			OpenTimeout:         20 * time.Millisecond,
			HalfOpenMaxRequests: 1,
		}),
	}

	for range 2 {
		_, _ = model.GetMeasurements(context.Background(), "asset-1", now.Add(-time.Minute), now)
	}

	_, err := model.GetMeasurements(context.Background(), "asset-1", now.Add(-time.Minute), now)
	if !errors.Is(err, query.ErrReadModelUnavailable) {
		t.Fatalf("expected read model unavailable error, got %v", err)
	}

	if executor.calls != 2 {
		t.Fatalf("expected query executor to be called twice before circuit opened, got %d", executor.calls)
	}
}

func TestReadModelGetMeasurementsHalfOpenClosesOnSuccess(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	executor := &countingQueryExecutor{
		err: errors.New("temporary failure"),
	}
	model := &ReadModel{
		bucket: "measurements",
		query:  executor,
		breaker: newCircuitBreaker(CircuitBreakerConfig{
			FailureThreshold:    1,
			OpenTimeout:         20 * time.Millisecond,
			HalfOpenMaxRequests: 1,
		}),
	}

	_, _ = model.GetMeasurements(context.Background(), "asset-1", now.Add(-time.Minute), now)

	waitForBreakerTimeout(t, 20*time.Millisecond)

	now = now.Add(2 * time.Second)
	executor.err = nil
	executor.records = []influxRecord{
		{Time: now, Field: "setpoint", Value: 10.0},
		{Time: now, Field: "active_power", Value: 9.0},
	}

	got, err := model.GetMeasurements(context.Background(), "asset-1", now.Add(-time.Minute), now)
	if err != nil {
		t.Fatalf("expected no error after half-open probe, got %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 point after circuit closed, got %d", len(got))
	}

	_, err = model.GetMeasurements(context.Background(), "asset-1", now.Add(-time.Minute), now)
	if err != nil {
		t.Fatalf("expected subsequent call to pass after breaker closed, got %v", err)
	}
}

func TestReadModelGetMeasurementsTripsCircuitOnIteratorError(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	wantErr := errors.New("stream failed")
	model := &ReadModel{
		bucket: "measurements",
		query: fakeQueryExecutor{
			iteratorErr: wantErr,
			records: []influxRecord{
				{Time: now, Field: "setpoint", Value: 10.0},
				{Time: now, Field: "active_power", Value: 9.0},
			},
		},
		breaker: newCircuitBreaker(CircuitBreakerConfig{
			FailureThreshold:    1,
			OpenTimeout:         20 * time.Millisecond,
			HalfOpenMaxRequests: 1,
		}),
	}

	_, err := model.GetMeasurements(context.Background(), "asset-1", now.Add(-time.Minute), now)
	if !errors.Is(err, query.ErrReadModelUnavailable) {
		t.Fatalf("expected read model unavailable error, got %v", err)
	}

	_, err = model.GetMeasurements(context.Background(), "asset-1", now.Add(-time.Minute), now)
	if !errors.Is(err, query.ErrReadModelUnavailable) {
		t.Fatalf("expected open circuit on second call, got %v", err)
	}
}

type fakeQueryExecutor struct {
	records     []influxRecord
	err         error
	iteratorErr error
}

func (f fakeQueryExecutor) Query(_ context.Context, _ string) (recordIterator, error) {
	if f.err != nil {
		return nil, f.err
	}

	return &sliceRecordIterator{records: f.records, err: f.iteratorErr}, nil
}

type countingQueryExecutor struct {
	records []influxRecord
	err     error
	calls   int
}

func (f *countingQueryExecutor) Query(_ context.Context, _ string) (recordIterator, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}

	return &sliceRecordIterator{records: f.records}, nil
}

type sliceRecordIterator struct {
	records []influxRecord
	index   int
	err     error
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
	return i.err
}
