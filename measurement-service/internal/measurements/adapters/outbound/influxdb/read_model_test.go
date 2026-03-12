package influxdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	getmeasurements "stellar/internal/measurements/application/get_measurements"
	"stellar/internal/measurements/domain"
)

type ReadModelSuite struct {
	suite.Suite
}

func TestReadModelSuite(t *testing.T) {
	suite.Run(t, new(ReadModelSuite))
}

func (s *ReadModelSuite) TestGetMeasurementsMapsInfluxRows() {
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
	s.Require().NoError(err)

	want := []domain.MeasurementPoint{
		{
			Timestamp:   base.Truncate(time.Second),
			Setpoint:    10,
			ActivePower: 9.5,
		},
	}

	s.Equal(want, got)
}

func (s *ReadModelSuite) TestGetMeasurementsHandlesEmptyResult() {
	model := &ReadModel{
		bucket: "measurements",
		query:  fakeQueryExecutor{},
	}

	got, err := model.GetMeasurements(context.Background(), "asset-1", time.Now().Add(-time.Minute), time.Now())
	s.Require().NoError(err)
	s.Empty(got)
}

func (s *ReadModelSuite) TestGetMeasurementsReturnsPointsOrderedByTimestamp() {
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
	s.Require().NoError(err)
	s.Len(got, 2)
	s.True(got[0].Timestamp.Before(got[1].Timestamp))
}

func (s *ReadModelSuite) TestGetMeasurementsSelectsLatestCompletePointWithinSecond() {
	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	testCases := []struct {
		name    string
		records []influxRecord
		want    []domain.MeasurementPoint
	}{
		{
			name: "later complete point wins",
			records: []influxRecord{
				{Time: base.Add(100 * time.Millisecond), Field: "setpoint", Value: 10.0},
				{Time: base.Add(100 * time.Millisecond), Field: "active_power", Value: 9.0},
				{Time: base.Add(700 * time.Millisecond), Field: "setpoint", Value: 13.0},
				{Time: base.Add(700 * time.Millisecond), Field: "active_power", Value: 12.5},
			},
			want: []domain.MeasurementPoint{
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
			want: []domain.MeasurementPoint{
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
			want: []domain.MeasurementPoint{
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
			want: []domain.MeasurementPoint{},
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			model := &ReadModel{
				bucket: "measurements",
				query: fakeQueryExecutor{
					records: tc.records,
				},
			}

			got, err := model.GetMeasurements(context.Background(), "asset-1", base.Add(-time.Minute), base.Add(time.Minute))
			s.Require().NoError(err)
			s.Equal(tc.want, got)
		})
	}
}

func (s *ReadModelSuite) TestGetMeasurementsReturnsExecutorError() {
	wantErr := errors.New("query failed")
	model := &ReadModel{
		bucket: "measurements",
		query:  fakeQueryExecutor{err: wantErr},
	}

	_, err := model.GetMeasurements(context.Background(), "asset-1", time.Now().Add(-time.Minute), time.Now())
	s.ErrorIs(err, wantErr)
}

func (s *ReadModelSuite) TestGetMeasurementsOpensCircuitAfterThreshold() {
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
	s.ErrorIs(err, getmeasurements.ErrReadModelUnavailable)
	s.Equal(2, executor.calls)
}

func (s *ReadModelSuite) TestGetMeasurementsHalfOpenClosesOnSuccess() {
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

	waitForBreakerTimeout(20 * time.Millisecond)

	now = now.Add(2 * time.Second)
	executor.err = nil
	executor.records = []influxRecord{
		{Time: now, Field: "setpoint", Value: 10.0},
		{Time: now, Field: "active_power", Value: 9.0},
	}

	got, err := model.GetMeasurements(context.Background(), "asset-1", now.Add(-time.Minute), now)
	s.Require().NoError(err)
	s.Len(got, 1)

	_, err = model.GetMeasurements(context.Background(), "asset-1", now.Add(-time.Minute), now)
	s.NoError(err)
}

func (s *ReadModelSuite) TestGetMeasurementsTripsCircuitOnIteratorError() {
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
	s.ErrorIs(err, getmeasurements.ErrReadModelUnavailable)

	_, err = model.GetMeasurements(context.Background(), "asset-1", now.Add(-time.Minute), now)
	s.ErrorIs(err, getmeasurements.ErrReadModelUnavailable)
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
