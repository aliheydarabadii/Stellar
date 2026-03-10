package ports

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	measurementsv1 "stellar/api/proto"
	"stellar/internal/measurements/app"
	"stellar/internal/measurements/app/query"
)

func TestGRPCServerGetMeasurementsMapsRequestAndResponse(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Second)
	readModel := &capturingReadModel{
		points: []query.MeasurementPoint{
			{
				Timestamp:   from,
				Setpoint:    11,
				ActivePower: 10.5,
			},
		},
	}

	application, err := app.New(readModel)
	if err != nil {
		t.Fatalf("expected valid app, got %v", err)
	}

	server := NewGRPCServer(application)

	resp, err := server.GetMeasurements(context.Background(), &measurementsv1.GetMeasurementsRequest{
		AssetId: "asset-1",
		From:    timestamppb.New(from),
		To:      timestamppb.New(to),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if readModel.assetID != "asset-1" {
		t.Fatalf("expected asset id asset-1, got %q", readModel.assetID)
	}

	if !readModel.from.Equal(from) {
		t.Fatalf("expected from %v, got %v", from, readModel.from)
	}

	if !readModel.to.Equal(to) {
		t.Fatalf("expected to %v, got %v", to, readModel.to)
	}

	if resp.GetAssetId() != "asset-1" {
		t.Fatalf("expected response asset id asset-1, got %q", resp.GetAssetId())
	}

	if len(resp.GetPoints()) != 1 {
		t.Fatalf("expected 1 point, got %d", len(resp.GetPoints()))
	}

	point := resp.GetPoints()[0]
	if point.GetSetpoint() != 11 {
		t.Fatalf("expected setpoint 11, got %v", point.GetSetpoint())
	}

	if point.GetActivePower() != 10.5 {
		t.Fatalf("expected active power 10.5, got %v", point.GetActivePower())
	}
}

func TestGRPCServerGetMeasurementsRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	application, err := app.New(&capturingReadModel{})
	if err != nil {
		t.Fatalf("expected valid app, got %v", err)
	}

	server := NewGRPCServer(application)

	_, err = server.GetMeasurements(context.Background(), &measurementsv1.GetMeasurementsRequest{
		AssetId: "",
		From:    timestamppb.New(now),
		To:      timestamppb.New(now),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestGRPCServerGetMeasurementsMapsReadModelUnavailable(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	application, err := app.New(&capturingReadModel{
		err: query.ErrReadModelUnavailable,
	})
	if err != nil {
		t.Fatalf("expected valid app, got %v", err)
	}

	server := NewGRPCServer(application)

	_, err = server.GetMeasurements(context.Background(), &measurementsv1.GetMeasurementsRequest{
		AssetId: "asset-1",
		From:    timestamppb.New(now),
		To:      timestamppb.New(now),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable, got %v", status.Code(err))
	}
}

func TestGRPCServerGetMeasurementsRejectsInvalidRequestShape(t *testing.T) {
	t.Parallel()

	application, err := app.New(&capturingReadModel{})
	if err != nil {
		t.Fatalf("expected valid app, got %v", err)
	}

	server := NewGRPCServer(application)

	now := time.Now().UTC()
	invalidTimestamp := &timestamppb.Timestamp{Seconds: 1, Nanos: 1_000_000_000}

	testCases := []struct {
		name string
		req  *measurementsv1.GetMeasurementsRequest
	}{
		{
			name: "nil request",
			req:  nil,
		},
		{
			name: "missing from",
			req: &measurementsv1.GetMeasurementsRequest{
				AssetId: "asset-1",
				To:      timestamppb.New(now),
			},
		},
		{
			name: "missing to",
			req: &measurementsv1.GetMeasurementsRequest{
				AssetId: "asset-1",
				From:    timestamppb.New(now),
			},
		},
		{
			name: "invalid protobuf timestamp",
			req: &measurementsv1.GetMeasurementsRequest{
				AssetId: "asset-1",
				From:    invalidTimestamp,
				To:      timestamppb.New(now),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := server.GetMeasurements(context.Background(), tc.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
			}
		})
	}
}

type capturingReadModel struct {
	assetID string
	from    time.Time
	to      time.Time
	points  []query.MeasurementPoint
	err     error
}

func (r *capturingReadModel) GetMeasurements(_ context.Context, assetID string, from, to time.Time) ([]query.MeasurementPoint, error) {
	r.assetID = assetID
	r.from = from
	r.to = to

	if r.err != nil {
		return nil, r.err
	}

	return r.points, nil
}
