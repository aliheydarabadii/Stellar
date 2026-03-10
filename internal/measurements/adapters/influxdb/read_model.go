// Package influxdb implements the InfluxDB-backed read model adapter.
package influxdb

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxapi "github.com/influxdata/influxdb-client-go/v2/api"

	"stellar/internal/measurements/app/query"
)

const measurementName = "asset_measurements"

var _ query.MeasurementsReadModel = (*ReadModel)(nil)

type ReadModel struct {
	bucket  string
	timeout time.Duration
	query   queryExecutor
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

func NewReadModel(client influxdb2.Client, org, bucket string, timeout time.Duration) *ReadModel {
	return &ReadModel{
		bucket:  bucket,
		timeout: timeout,
		query: influxQueryExecutor{
			queryAPI: client.QueryAPI(org),
		},
	}
}

func (r *ReadModel) GetMeasurements(ctx context.Context, assetID string, from, to time.Time) ([]query.MeasurementPoint, error) {
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	records, err := r.query.Query(ctx, buildMeasurementsQuery(r.bucket, assetID, from, to))
	if err != nil {
		return nil, fmt.Errorf("query influxdb: %w", err)
	}

	points, err := mapRecordsToPoints(records)
	if err != nil {
		return nil, fmt.Errorf("map influxdb records: %w", err)
	}

	return points, nil
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
