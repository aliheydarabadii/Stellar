//go:build integration

package ports

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	measurementsv1 "stellar/api/proto"
	"stellar/internal/measurements/adapters/influxdb"
	"stellar/internal/measurements/app"
	"stellar/internal/measurements/testsupport"
)

func TestGRPCServerIntegrationGetMeasurements(t *testing.T) {
	influx := testsupport.StartInfluxDB(t)

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	testsupport.SeedMeasurements(t, influx.Client, influx.Org, influx.Bucket, []testsupport.MeasurementSeed{
		{
			AssetID:     "asset-1",
			Timestamp:   base,
			Setpoint:    testsupport.Float64Ptr(10),
			ActivePower: testsupport.Float64Ptr(9),
		},
		{
			AssetID:   "asset-1",
			Timestamp: base.Add(time.Second + 100*time.Millisecond),
			Setpoint:  testsupport.Float64Ptr(20),
		},
		{
			AssetID:     "asset-1",
			Timestamp:   base.Add(time.Second + 100*time.Millisecond),
			ActivePower: testsupport.Float64Ptr(19),
		},
		{
			AssetID:     "asset-2",
			Timestamp:   base,
			Setpoint:    testsupport.Float64Ptr(999),
			ActivePower: testsupport.Float64Ptr(998),
		},
	})

	readModel := influxdb.NewReadModel(influx.Client, influx.Org, influx.Bucket, 10*time.Second, influxdb.CircuitBreakerConfig{})
	application, err := app.New(readModel)
	if err != nil {
		t.Fatalf("create application: %v", err)
	}

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	measurementsv1.RegisterMeasurementServiceServer(server, NewGRPCServer(application))
	t.Cleanup(server.Stop)

	go func() {
		_ = server.Serve(listener)
	}()

	connCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(connCtx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial gRPC server: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close gRPC client conn: %v", err)
		}
	})

	client := measurementsv1.NewMeasurementServiceClient(conn)

	requestCtx, requestCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer requestCancel()

	resp, err := client.GetMeasurements(requestCtx, &measurementsv1.GetMeasurementsRequest{
		AssetId: "asset-1",
		From:    timestamppb.New(base),
		To:      timestamppb.New(base.Add(2 * time.Second)),
	})
	if err != nil {
		t.Fatalf("get measurements via gRPC: %v", err)
	}

	if resp.GetAssetId() != "asset-1" {
		t.Fatalf("expected asset-1, got %q", resp.GetAssetId())
	}
	if len(resp.GetPoints()) != 2 {
		t.Fatalf("expected 2 points, got %d", len(resp.GetPoints()))
	}
	if !resp.GetPoints()[0].GetTimestamp().AsTime().Equal(base) {
		t.Fatalf("expected first timestamp %s, got %s", base, resp.GetPoints()[0].GetTimestamp().AsTime())
	}
	if resp.GetPoints()[1].GetSetpoint() != 20 || resp.GetPoints()[1].GetActivePower() != 19 {
		t.Fatalf("expected second point values 20/19, got %v/%v", resp.GetPoints()[1].GetSetpoint(), resp.GetPoints()[1].GetActivePower())
	}

	_, err = client.GetMeasurements(requestCtx, &measurementsv1.GetMeasurementsRequest{
		AssetId: "asset-1",
		To:      timestamppb.New(base),
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for missing from, got %v", status.Code(err))
	}
}
