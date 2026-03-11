//go:build integration

package testsupport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	TestInfluxOrg      = "local"
	TestInfluxBucket   = "telemetry"
	TestInfluxToken    = "dev-token"
	testInfluxUsername = "admin"
	testInfluxPassword = "password12345"
	testInfluxImage    = "influxdb:2.7"
)

type TestInflux struct {
	URL       string
	Org       string
	Bucket    string
	Token     string
	Client    influxdb2.Client
	Container testcontainers.Container
}

type MeasurementSeed struct {
	AssetID     string
	Timestamp   time.Time
	Setpoint    *float64
	ActivePower *float64
}

func StartInfluxDB(t testing.TB) *TestInflux {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        testInfluxImage,
			ExposedPorts: []string{"8086/tcp"},
			Env: map[string]string{
				"DOCKER_INFLUXDB_INIT_MODE":        "setup",
				"DOCKER_INFLUXDB_INIT_USERNAME":    testInfluxUsername,
				"DOCKER_INFLUXDB_INIT_PASSWORD":    testInfluxPassword,
				"DOCKER_INFLUXDB_INIT_ORG":         TestInfluxOrg,
				"DOCKER_INFLUXDB_INIT_BUCKET":      TestInfluxBucket,
				"DOCKER_INFLUXDB_INIT_ADMIN_TOKEN": TestInfluxToken,
			},
			WaitingFor: wait.ForListeningPort("8086/tcp"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start influxdb container: %v", err)
	}

	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Errorf("terminate influxdb container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get influxdb container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "8086/tcp")
	if err != nil {
		t.Fatalf("get influxdb container port: %v", err)
	}

	baseURL := fmt.Sprintf("http://%s:%s", host, port.Port())
	waitForInfluxHealth(t, baseURL)

	client := influxdb2.NewClient(baseURL, TestInfluxToken)
	t.Cleanup(client.Close)

	return &TestInflux{
		URL:       baseURL,
		Org:       TestInfluxOrg,
		Bucket:    TestInfluxBucket,
		Token:     TestInfluxToken,
		Client:    client,
		Container: container,
	}
}

func SeedMeasurements(t testing.TB, client influxdb2.Client, org, bucket string, seeds []MeasurementSeed) {
	t.Helper()

	writeAPI := client.WriteAPIBlocking(org, bucket)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, seed := range seeds {
		fields := make(map[string]any, 2)
		if seed.Setpoint != nil {
			fields["setpoint"] = *seed.Setpoint
		}
		if seed.ActivePower != nil {
			fields["active_power"] = *seed.ActivePower
		}
		if len(fields) == 0 {
			t.Fatalf("seed for asset %q at %s has no fields", seed.AssetID, seed.Timestamp)
		}

		point := influxdb2.NewPoint(
			"asset_measurements",
			map[string]string{"asset_id": seed.AssetID},
			fields,
			seed.Timestamp.UTC(),
		)

		if err := writeAPI.WritePoint(ctx, point); err != nil {
			t.Fatalf("write seed point for asset %q at %s: %v", seed.AssetID, seed.Timestamp, err)
		}
	}
}

func Float64Ptr(value float64) *float64 {
	return &value
}

func waitForInfluxHealth(t testing.TB, baseURL string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Minute)
	httpClient := &http.Client{Timeout: 2 * time.Second}
	var lastErr error

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/health", nil)
		if err != nil {
			t.Fatalf("build health request: %v", err)
		}

		resp, err := httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return
		}

		if err == nil {
			lastErr = fmt.Errorf("health endpoint returned status %d", resp.StatusCode)
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		} else {
			lastErr = err
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("wait for influxdb health: %v", lastErr)
}
