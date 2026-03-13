// Package influxdb implements the InfluxDB-backed read model adapter.
package influxdb

import (
	"context"
	"fmt"
	"stellar/internal/measurements"
	getmeasurements "stellar/internal/measurements/application"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxapi "github.com/influxdata/influxdb-client-go/v2/api"
)

const measurementName = "asset_measurements"

var _ measurements.MeasurementsReadModel = (*ReadModel)(nil)

type ReadModel struct {
	bucket  string
	timeout time.Duration
	query   queryExecutor
	breaker *circuitBreaker
}

type queryExecutor interface {
	Query(ctx context.Context, flux string) (recordIterator, error)
}

type recordIterator interface {
	Next() bool
	Record() influxRecord
	Err() error
}

type influxRecord struct {
	Time  time.Time
	Field string
	Value any
}

func NewReadModel(client influxdb2.Client, org, bucket string, timeout time.Duration, breakerConfig CircuitBreakerConfig) *ReadModel {
	return &ReadModel{
		bucket:  bucket,
		timeout: timeout,
		query: influxQueryExecutor{
			queryAPI: client.QueryAPI(org),
		},
		breaker: newCircuitBreaker(breakerConfig),
	}
}

func (r *ReadModel) GetMeasurements(ctx context.Context, assetID string, from, to time.Time) ([]measurements.MeasurementPoint, error) {
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	loadMeasurements := func() ([]measurements.MeasurementPoint, error) {
		records, err := r.query.Query(ctx, buildMeasurementsQuery(r.bucket, assetID, from, to))
		if err != nil {
			return nil, fmt.Errorf("%w: query influxdb: %w", getmeasurements.ErrReadModelUnavailable, err)
		}

		points, err := mapRecordsToPoints(records)
		if err != nil {
			return nil, fmt.Errorf("map influxdb records: %w", err)
		}

		if err := records.Err(); err != nil {
			return nil, fmt.Errorf("%w: iterate influxdb records: %w", getmeasurements.ErrReadModelUnavailable, err)
		}

		return points, nil
	}

	if r.breaker != nil {
		done, err := r.breaker.allow()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", getmeasurements.ErrReadModelUnavailable, err)
		}

		points, err := loadMeasurements()
		done(err)
		return points, err
	}

	return loadMeasurements()
}

func buildMeasurementsQuery(bucket, assetID string, from, to time.Time) string {
	return fmt.Sprintf(`
from(bucket: %q)
  |> range(start: time(v: %q), stop: time(v: %q))
  |> filter(fn: (r) => r._measurement == %q)
  |> filter(fn: (r) => r.asset_id == %q)
  |> filter(fn: (r) => r._field == "setpoint" or r._field == "active_power")
  |> keep(columns: ["_time", "_field", "_value"])
  |> sort(columns: ["_time"], desc: false)
`, bucket, from.UTC().Format(time.RFC3339Nano), to.UTC().Add(time.Nanosecond).Format(time.RFC3339Nano), measurementName, assetID)
}

type influxQueryExecutor struct {
	queryAPI influxapi.QueryAPI
}

func (e influxQueryExecutor) Query(ctx context.Context, flux string) (recordIterator, error) {
	result, err := e.queryAPI.Query(ctx, flux)
	if err != nil {
		return nil, err
	}

	return &influxResultIterator{result: result}, nil
}

type influxResultIterator struct {
	result *influxapi.QueryTableResult
}

func (i *influxResultIterator) Next() bool {
	return i.result.Next()
}

func (i *influxResultIterator) Record() influxRecord {
	record := i.result.Record()
	return influxRecord{
		Time:  record.Time(),
		Field: record.Field(),
		Value: record.Value(),
	}
}

func (i *influxResultIterator) Err() error {
	return i.result.Err()
}
