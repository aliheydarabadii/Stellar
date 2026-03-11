package influxdb_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"stellar/internal/telemetry/adapters/influxdb"
	"stellar/internal/telemetry/domain"
)

const (
	testInfluxImage    = "influxdb:2.7"
	testInfluxUsername = "integration-user"
	testInfluxPassword = "integration-password"
	testInfluxOrg      = "integration-org"
	testInfluxBucket   = "integration-bucket"
	testInfluxToken    = "integration-admin-token"
)

type MeasurementRepositoryIntegrationTestSuite struct {
	suite.Suite
	container   testcontainers.Container
	queryClient influxdb2.Client
	baseURL     string
}

type persistedMeasurement struct {
	MeasurementName string
	AssetID         string
	AssetType       string
	Setpoint        float64
	ActivePower     float64
	CollectedAt     time.Time
}

func TestMeasurementRepositoryIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(MeasurementRepositoryIntegrationTestSuite))
}

func (s *MeasurementRepositoryIntegrationTestSuite) SetupSuite() {
	testcontainers.SkipIfProviderIsNotHealthy(s.T())

	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        testInfluxImage,
			ExposedPorts: []string{"8086/tcp"},
			Env: map[string]string{
				"DOCKER_INFLUXDB_INIT_MODE":        "setup",
				"DOCKER_INFLUXDB_INIT_USERNAME":    testInfluxUsername,
				"DOCKER_INFLUXDB_INIT_PASSWORD":    testInfluxPassword,
				"DOCKER_INFLUXDB_INIT_ORG":         testInfluxOrg,
				"DOCKER_INFLUXDB_INIT_BUCKET":      testInfluxBucket,
				"DOCKER_INFLUXDB_INIT_ADMIN_TOKEN": testInfluxToken,
			},
			WaitingFor: wait.ForHTTP("/health").
				WithPort("8086/tcp").
				WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
				WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	s.Require().NoError(err)
	s.container = container

	host, err := container.Host(ctx)
	s.Require().NoError(err)

	port, err := container.MappedPort(ctx, "8086/tcp")
	s.Require().NoError(err)

	s.baseURL = fmt.Sprintf("http://%s:%s", host, port.Port())
	s.queryClient = influxdb2.NewClient(s.baseURL, testInfluxToken)

	s.waitUntilInfluxSetupCompletes()
}

func (s *MeasurementRepositoryIntegrationTestSuite) TearDownSuite() {
	if s.queryClient != nil {
		s.queryClient.Close()
	}

	if s.container != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		s.Require().NoError(s.container.Terminate(ctx))
	}
}

func (s *MeasurementRepositoryIntegrationTestSuite) TestSavePersistsMeasurementToRealInfluxDB() {
	repository, err := influxdb.NewMeasurementRepositoryWithConfig(influxdb.Config{
		BaseURL:   s.baseURL,
		Org:       testInfluxOrg,
		Bucket:    testInfluxBucket,
		Token:     testInfluxToken,
		Timeout:   10 * time.Second,
		WriteMode: influxdb.WriteModeBlocking,
	}, influxdb.NewPointMapperWithAssetType(string(domain.SolarPanelType)))
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		s.Require().NoError(repository.Close())
	})

	collectedAt := time.Date(2026, time.March, 10, 12, 0, 0, 123456789, time.UTC)
	measurement, err := domain.NewMeasurement(domain.AssetID("integration-asset-1"), 100, 55, collectedAt)
	s.Require().NoError(err)

	ctx := context.Background()
	s.Require().NoError(repository.Save(ctx, measurement))

	var persisted []persistedMeasurement
	s.Require().Eventually(func() bool {
		var queryErr error
		persisted, queryErr = s.queryMeasurements(ctx, measurement.AssetID.String(), collectedAt.Add(-time.Minute), collectedAt.Add(time.Minute))
		return queryErr == nil && len(persisted) > 0
	}, 15*time.Second, 500*time.Millisecond, "expected saved point to become queryable")

	s.Require().Len(persisted, 1)

	record := persisted[0]
	s.Assert().Equal("asset_measurements", record.MeasurementName)
	s.Assert().Equal(measurement.AssetID.String(), record.AssetID)
	s.Assert().Equal(string(domain.SolarPanelType), record.AssetType)
	s.Assert().Equal(measurement.Setpoint, record.Setpoint)
	s.Assert().Equal(measurement.ActivePower, record.ActivePower)
	s.Assert().Equal(measurement.CollectedAt, record.CollectedAt)
}

func (s *MeasurementRepositoryIntegrationTestSuite) TestBatchModeSavePersistsMeasurementToRealInfluxDB() {
	repository, err := influxdb.NewMeasurementRepositoryWithConfig(influxdb.Config{
		BaseURL:       s.baseURL,
		Org:           testInfluxOrg,
		Bucket:        testInfluxBucket,
		Token:         testInfluxToken,
		Timeout:       10 * time.Second,
		WriteMode:     influxdb.WriteModeBatch,
		BatchSize:     10,
		FlushInterval: 25 * time.Millisecond,
	}, influxdb.NewPointMapperWithAssetType(string(domain.SolarPanelType)))
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		s.Require().NoError(repository.Close())
	})

	collectedAt := time.Date(2026, time.March, 10, 12, 1, 0, 987654321, time.UTC)
	measurement, err := domain.NewMeasurement(domain.AssetID("integration-asset-batch-1"), 120, 65, collectedAt)
	s.Require().NoError(err)

	ctx := context.Background()
	s.Require().NoError(repository.Save(ctx, measurement))

	persisted, err := s.queryMeasurements(ctx, measurement.AssetID.String(), collectedAt.Add(-time.Minute), collectedAt.Add(time.Minute))
	s.Require().NoError(err)
	s.Require().Len(persisted, 1)

	record := persisted[0]
	s.Assert().Equal("asset_measurements", record.MeasurementName)
	s.Assert().Equal(measurement.AssetID.String(), record.AssetID)
	s.Assert().Equal(string(domain.SolarPanelType), record.AssetType)
	s.Assert().Equal(measurement.Setpoint, record.Setpoint)
	s.Assert().Equal(measurement.ActivePower, record.ActivePower)
	s.Assert().Equal(measurement.CollectedAt, record.CollectedAt)
}

func (s *MeasurementRepositoryIntegrationTestSuite) TestBatchModePersistsMultipleMeasurementsWhenBatchSizeIsReached() {
	repository, err := influxdb.NewMeasurementRepositoryWithConfig(influxdb.Config{
		BaseURL:       s.baseURL,
		Org:           testInfluxOrg,
		Bucket:        testInfluxBucket,
		Token:         testInfluxToken,
		Timeout:       10 * time.Second,
		WriteMode:     influxdb.WriteModeBatch,
		BatchSize:     3,
		FlushInterval: time.Hour,
	}, influxdb.NewPointMapperWithAssetType(string(domain.SolarPanelType)))
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		s.Require().NoError(repository.Close())
	})

	baseCollectedAt := time.Date(2026, time.March, 10, 12, 2, 0, 0, time.UTC)
	measurements := []domain.Measurement{
		s.newMeasurement("integration-batch-asset-1", 130, 70, baseCollectedAt),
		s.newMeasurement("integration-batch-asset-2", 140, 80, baseCollectedAt.Add(100*time.Millisecond)),
		s.newMeasurement("integration-batch-asset-3", 150, 90, baseCollectedAt.Add(200*time.Millisecond)),
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(measurements))
	for _, measurement := range measurements {
		wg.Add(1)
		go func(measurement domain.Measurement) {
			defer wg.Done()
			errCh <- repository.Save(context.Background(), measurement)
		}(measurement)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		s.Require().NoError(err)
	}

	var persisted []persistedMeasurement
	s.Require().Eventually(func() bool {
		var queryErr error
		persisted, queryErr = s.queryMeasurementsInRange(
			context.Background(),
			baseCollectedAt.Add(-time.Second),
			baseCollectedAt.Add(time.Second),
		)
		if queryErr != nil {
			return false
		}

		return len(persisted) == len(measurements)
	}, 15*time.Second, 500*time.Millisecond, "expected all batched measurements to become queryable")

	s.Require().Len(persisted, len(measurements))

	expected := make(map[string]domain.Measurement, len(measurements))
	for _, measurement := range measurements {
		expected[measurement.AssetID.String()] = measurement
	}

	for _, record := range persisted {
		want, ok := expected[record.AssetID]
		s.Require().True(ok, "unexpected asset id %s", record.AssetID)

		s.Equal("asset_measurements", record.MeasurementName)
		s.Equal(string(domain.SolarPanelType), record.AssetType)
		s.Equal(want.Setpoint, record.Setpoint)
		s.Equal(want.ActivePower, record.ActivePower)
		s.Equal(want.CollectedAt, record.CollectedAt)
	}
}

func (s *MeasurementRepositoryIntegrationTestSuite) waitUntilInfluxSetupCompletes() {
	queryAPI := s.queryClient.QueryAPI(testInfluxOrg)

	s.Require().Eventually(func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		result, err := queryAPI.Query(
			ctx,
			fmt.Sprintf(`buckets() |> filter(fn: (r) => r.name == %q) |> limit(n: 1)`, testInfluxBucket),
		)
		if err != nil {
			return false
		}
		defer result.Close()

		for result.Next() {
			return fmt.Sprint(result.Record().ValueByKey("name")) == testInfluxBucket
		}

		return false
	}, 30*time.Second, 500*time.Millisecond, "expected InfluxDB setup to complete")
}

func (s *MeasurementRepositoryIntegrationTestSuite) queryMeasurements(
	ctx context.Context,
	assetID string,
	start time.Time,
	stop time.Time,
) ([]persistedMeasurement, error) {
	filter := fmt.Sprintf(`|> filter(fn: (r) => r.asset_id == %q)`, assetID)
	return s.queryMeasurementsWithFilter(ctx, start, stop, filter)
}

func (s *MeasurementRepositoryIntegrationTestSuite) queryMeasurementsInRange(
	ctx context.Context,
	start time.Time,
	stop time.Time,
) ([]persistedMeasurement, error) {
	return s.queryMeasurementsWithFilter(ctx, start, stop, "")
}

func (s *MeasurementRepositoryIntegrationTestSuite) queryMeasurementsWithFilter(
	ctx context.Context,
	start time.Time,
	stop time.Time,
	filter string,
) ([]persistedMeasurement, error) {
	queryAPI := s.queryClient.QueryAPI(testInfluxOrg)
	query := fmt.Sprintf(`
from(bucket: %q)
	|> range(start: time(v: %q), stop: time(v: %q))
	|> filter(fn: (r) => r._measurement == %q)
	%s
	|> pivot(rowKey: ["_time", "_measurement", "asset_id", "asset_type"], columnKey: ["_field"], valueColumn: "_value")
`, testInfluxBucket, start.Format(time.RFC3339Nano), stop.Format(time.RFC3339Nano), "asset_measurements", filter)

	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer result.Close()

	measurements := make([]persistedMeasurement, 0, 1)
	for result.Next() {
		record := result.Record()

		setpoint, err := toFloat64(record.ValueByKey("setpoint"))
		if err != nil {
			return nil, err
		}

		activePower, err := toFloat64(record.ValueByKey("active_power"))
		if err != nil {
			return nil, err
		}

		measurements = append(measurements, persistedMeasurement{
			MeasurementName: record.Measurement(),
			AssetID:         fmt.Sprint(record.ValueByKey("asset_id")),
			AssetType:       fmt.Sprint(record.ValueByKey("asset_type")),
			Setpoint:        setpoint,
			ActivePower:     activePower,
			CollectedAt:     record.Time(),
		})
	}

	if err := result.Err(); err != nil {
		return nil, err
	}

	return measurements, nil
}

func (s *MeasurementRepositoryIntegrationTestSuite) newMeasurement(assetID string, setpoint, activePower float64, collectedAt time.Time) domain.Measurement {
	measurement, err := domain.NewMeasurement(domain.AssetID(assetID), setpoint, activePower, collectedAt)
	s.Require().NoError(err)

	return measurement
}

func toFloat64(value interface{}) (float64, error) {
	switch typed := value.(type) {
	case float64:
		return typed, nil
	case float32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case int:
		return float64(typed), nil
	default:
		return 0, fmt.Errorf("unexpected numeric value type %T", value)
	}
}
