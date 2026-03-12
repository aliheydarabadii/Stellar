// Package influxdb implements the InfluxDB measurement repository adapter.
package influxdb

import (
	"context"
	"fmt"
	"sync"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	collecttelemetry "stellar/internal/telemetry/application/collect_telemetry"
	"stellar/internal/telemetry/domain"
)

const (
	defaultBaseURL       = "http://127.0.0.1:8086"
	defaultOrg           = "local"
	defaultBucket        = "telemetry"
	defaultToken         = "dev-token"
	defaultTimeout       = 5 * time.Second
	defaultBatchSize     = 100
	defaultFlushInterval = 200 * time.Millisecond
)

type WriteMode string

const (
	WriteModeBlocking WriteMode = "blocking"
	WriteModeBatch    WriteMode = "batch"
)

type Config struct {
	BaseURL       string
	Org           string
	Bucket        string
	Token         string
	Timeout       time.Duration
	LogLevel      uint
	WriteMode     WriteMode
	BatchSize     uint
	FlushInterval time.Duration
}

type MeasurementRepository struct {
	client    influxdb2.Client
	writer    api.WriteAPIBlocking
	mapper    *PointMapper
	writeMode WriteMode
	batcher   *batchWriter

	closeOnce sync.Once
	closeErr  error
}

func NewMeasurementRepository(mapper *PointMapper) (*MeasurementRepository, error) {
	return NewMeasurementRepositoryWithConfig(DefaultConfig(), mapper)
}

func NewMeasurementRepositoryWithConfig(config Config, mapper *PointMapper) (*MeasurementRepository, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	if mapper == nil {
		mapper = NewPointMapper()
	}

	options := influxdb2.DefaultOptions()
	options.SetHTTPRequestTimeout(uint(config.Timeout / time.Second))
	options.SetLogLevel(config.LogLevel)

	client := influxdb2.NewClientWithOptions(config.BaseURL, config.Token, options)
	writer := client.WriteAPIBlocking(config.Org, config.Bucket)

	var batcher *batchWriter
	if config.WriteMode == WriteModeBatch {
		batcher = newBatchWriter(
			writer,
			config.Timeout,
			effectiveBatchSize(config.BatchSize),
			effectiveFlushInterval(config.FlushInterval),
		)
	}

	return &MeasurementRepository{
		client:    client,
		writer:    writer,
		mapper:    mapper,
		writeMode: config.WriteMode,
		batcher:   batcher,
	}, nil
}

func DefaultConfig() Config {
	return Config{
		BaseURL:   defaultBaseURL,
		Org:       defaultOrg,
		Bucket:    defaultBucket,
		Token:     defaultToken,
		Timeout:   defaultTimeout,
		WriteMode: WriteModeBlocking,
	}
}

func (r *MeasurementRepository) Save(ctx context.Context, measurement domain.Measurement) error {
	point := r.mapper.Map(measurement)

	if r.writeMode == WriteModeBatch && r.batcher != nil {
		if err := r.batcher.WritePoint(ctx, toInfluxPoint(point)); err != nil {
			return fmt.Errorf("write influxdb point: %w", err)
		}
		return nil
	}

	if err := r.writer.WritePoint(ctx, toInfluxPoint(point)); err != nil {
		return fmt.Errorf("write influxdb point: %w", err)
	}

	return nil
}

func (r *MeasurementRepository) Close() error {
	r.closeOnce.Do(func() {
		if r.batcher != nil {
			r.closeErr = r.batcher.Close()
		}

		if r.client != nil {
			r.client.Close()
		}
	})

	return r.closeErr
}

var _ collecttelemetry.MeasurementRepository = (*MeasurementRepository)(nil)

func validateConfig(config Config) error {
	switch {
	case config.BaseURL == "":
		return fmt.Errorf("influxdb config: %w", ErrEmptyBaseURL)
	case config.Org == "":
		return fmt.Errorf("influxdb config: %w", ErrEmptyOrg)
	case config.Bucket == "":
		return fmt.Errorf("influxdb config: %w", ErrEmptyBucket)
	case config.Token == "":
		return fmt.Errorf("influxdb config: %w", ErrEmptyToken)
	case config.Timeout <= 0:
		return fmt.Errorf("influxdb config: %w", ErrInvalidTimeout)
	case config.WriteMode == "":
		return fmt.Errorf("influxdb config: %w", ErrInvalidWriteMode)
	case config.WriteMode != WriteModeBlocking && config.WriteMode != WriteModeBatch:
		return fmt.Errorf("influxdb config: %w: %q", ErrInvalidWriteMode, config.WriteMode)
	case config.FlushInterval < 0:
		return fmt.Errorf("influxdb config: flush interval must not be negative")
	}

	return nil
}

func toInfluxPoint(point Point) *write.Point {
	tags := make(map[string]string, 2)
	tags["asset_id"] = point.Tags.AssetID
	if point.Tags.AssetType != "" {
		tags["asset_type"] = point.Tags.AssetType
	}

	fields := make(map[string]interface{}, 2)
	fields["setpoint"] = point.Fields.Setpoint
	fields["active_power"] = point.Fields.ActivePower

	return influxdb2.NewPoint(
		point.Name,
		tags,
		fields,
		point.Timestamp,
	)
}

func effectiveBatchSize(configured uint) int {
	if configured == 0 {
		return defaultBatchSize
	}

	return int(configured)
}

func effectiveFlushInterval(configured time.Duration) time.Duration {
	if configured <= 0 {
		return defaultFlushInterval
	}

	return configured
}

type writeRequest struct {
	ctx    context.Context
	point  *write.Point
	result chan error
}

type batchWriter struct {
	writer        api.WriteAPIBlocking
	timeout       time.Duration
	batchSize     int
	flushInterval time.Duration
	requests      chan writeRequest
	closeCh       chan chan error

	enqueueMu sync.Mutex
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func newBatchWriter(writer api.WriteAPIBlocking, timeout time.Duration, batchSize int, flushInterval time.Duration) *batchWriter {
	b := &batchWriter{
		writer:        writer,
		timeout:       timeout,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		requests:      make(chan writeRequest, batchSize),
		closeCh:       make(chan chan error),
	}

	go b.run()

	return b
}

func (b *batchWriter) WritePoint(ctx context.Context, point *write.Point) error {
	request := writeRequest{
		ctx:    ctx,
		point:  point,
		result: make(chan error, 1),
	}

	b.enqueueMu.Lock()
	if b.closed {
		b.enqueueMu.Unlock()
		return ErrRepositoryClosed
	}

	select {
	case <-ctx.Done():
		b.enqueueMu.Unlock()
		return ctx.Err()
	case b.requests <- request:
		b.enqueueMu.Unlock()
	}

	select {
	case err := <-request.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *batchWriter) Close() error {
	b.closeOnce.Do(func() {
		b.enqueueMu.Lock()
		b.closed = true
		b.enqueueMu.Unlock()

		result := make(chan error, 1)
		b.closeCh <- result
		b.closeErr = <-result
	})

	return b.closeErr
}

func (b *batchWriter) run() {
	timer := time.NewTimer(b.flushInterval)
	stopTimer(timer)

	batch := make([]writeRequest, 0, b.batchSize)

	for {
		if len(batch) == 0 {
			select {
			case result := <-b.closeCh:
				result <- b.flush(b.drainPending(batch))
				return
			case request := <-b.requests:
				if request.ctx.Err() != nil {
					request.result <- request.ctx.Err()
					continue
				}

				batch = append(batch, request)
				resetTimer(timer, b.flushInterval)
			}

			continue
		}

		select {
		case result := <-b.closeCh:
			stopTimer(timer)
			result <- b.flush(b.drainPending(batch))
			return
		case request := <-b.requests:
			if request.ctx.Err() != nil {
				request.result <- request.ctx.Err()
				continue
			}

			batch = append(batch, request)
			if len(batch) >= b.batchSize {
				stopTimer(timer)
				flushErr := b.flush(batch)
				batch = batch[:0]
				if flushErr == nil {
					continue
				}
			}
		case <-timer.C:
			_ = b.flush(batch)
			batch = batch[:0]
		}

		if len(batch) == 0 {
			stopTimer(timer)
		}
	}
}

func (b *batchWriter) flush(batch []writeRequest) error {
	if len(batch) == 0 {
		return nil
	}

	points := make([]*write.Point, 0, len(batch))
	active := make([]writeRequest, 0, len(batch))
	for _, request := range batch {
		if request.ctx.Err() != nil {
			request.result <- request.ctx.Err()
			continue
		}

		points = append(points, request.point)
		active = append(active, request)
	}

	if len(active) == 0 {
		return nil
	}

	flushCtx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	err := b.writer.WritePoint(flushCtx, points...)
	for _, request := range active {
		request.result <- err
	}

	return err
}

func (b *batchWriter) drainPending(batch []writeRequest) []writeRequest {
	for {
		select {
		case request := <-b.requests:
			batch = append(batch, request)
		default:
			return batch
		}
	}
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	stopTimer(timer)
	timer.Reset(duration)
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
