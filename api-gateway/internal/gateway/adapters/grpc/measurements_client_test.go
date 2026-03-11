package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"api_gateway/internal/gateway/requestctx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	measurementsv1 "stellar/api/proto"
)

const bufConnSize = 1024 * 1024

func TestMeasurementsClientGetMeasurementsPropagatesRequestMetadata(t *testing.T) {
	t.Parallel()

	listener := bufconn.Listen(bufConnSize)
	server := grpc.NewServer()
	testServer := &capturingMeasurementService{}
	measurementsv1.RegisterMeasurementServiceServer(server, testServer)

	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	client := &MeasurementsClient{
		conn:   conn,
		client: measurementsv1.NewMeasurementServiceClient(conn),
	}

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	ctx := requestctx.WithValues(context.Background(), "req-123", "corr-123")

	_, err = client.GetMeasurements(ctx, "asset-1", base, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("get measurements: %v", err)
	}

	if testServer.requestID != "req-123" {
		t.Fatalf("expected request id %q, got %q", "req-123", testServer.requestID)
	}
	if testServer.correlationID != "corr-123" {
		t.Fatalf("expected correlation id %q, got %q", "corr-123", testServer.correlationID)
	}
}

type capturingMeasurementService struct {
	measurementsv1.UnimplementedMeasurementServiceServer
	requestID     string
	correlationID string
}

func (s *capturingMeasurementService) GetMeasurements(ctx context.Context, req *measurementsv1.GetMeasurementsRequest) (*measurementsv1.GetMeasurementsResponse, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		s.requestID = firstMetadataValue(md, requestctx.RequestIDHeader)
		s.correlationID = firstMetadataValue(md, requestctx.CorrelationIDHeader)
	}

	return &measurementsv1.GetMeasurementsResponse{
		AssetId: req.GetAssetId(),
		Points: []*measurementsv1.MeasurementPoint{
			{
				Timestamp:   timestamppb.New(req.GetFrom().AsTime()),
				Setpoint:    10,
				ActivePower: 9,
			},
		},
	}, nil
}

func firstMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}

	return values[0]
}
