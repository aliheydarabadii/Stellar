package influxdb

import (
	"context"
	"fmt"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"stellar/internal/telemetry/domain"
)

const (
	defaultBaseURL = "http://127.0.0.1:8086"
	defaultOrg     = "local"
	defaultBucket  = "telemetry"
	defaultToken   = "dev-token"
	defaultTimeout = 5 * time.Second
)

type Config struct {
	BaseURL string
	Org     string
	Bucket  string
	Token   string
	Timeout time.Duration
}

type MeasurementRepository struct {
	client influxdb2.Client
	writer api.WriteAPIBlocking
	mapper *PointMapper
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

	client := influxdb2.NewClientWithOptions(config.BaseURL, config.Token, options)
	writer := client.WriteAPIBlocking(config.Org, config.Bucket)

	return &MeasurementRepository{
		client: client,
		writer: writer,
		mapper: mapper,
	}, nil
}

func DefaultConfig() Config {
	return Config{
		BaseURL: defaultBaseURL,
		Org:     defaultOrg,
		Bucket:  defaultBucket,
		Token:   defaultToken,
		Timeout: defaultTimeout,
	}
}

func (r *MeasurementRepository) Save(ctx context.Context, measurement domain.Measurement) error {
	point := r.mapper.Map(measurement)

	if err := r.writer.WritePoint(ctx, toInfluxPoint(point)); err != nil {
		return fmt.Errorf("write influxdb point: %w", err)
	}

	return nil
}

func (r *MeasurementRepository) Close() {
	if r.client != nil {
		r.client.Close()
	}
}

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
