package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	measurementsv1 "stellar/api/proto"

	"api_gateway/internal/gateway/app/query"
)

type MeasurementsClient struct {
	conn   *grpcpkg.ClientConn
	client measurementsv1.MeasurementServiceClient
}

func Dial(ctx context.Context, address string) (*MeasurementsClient, error) {
	if strings.TrimSpace(address) == "" {
		return nil, errors.New("measurement service gRPC address is required")
	}

	conn, err := grpcpkg.DialContext(
		ctx,
		address,
		grpcpkg.WithTransportCredentials(insecure.NewCredentials()),
		grpcpkg.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial measurement service: %w", err)
	}

	return &MeasurementsClient{
		conn:   conn,
		client: measurementsv1.NewMeasurementServiceClient(conn),
	}, nil
}

func (c *MeasurementsClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func (c *MeasurementsClient) GetMeasurements(ctx context.Context, assetID string, from, to time.Time) (query.MeasurementSeries, error) {
	if c == nil || c.client == nil {
		return query.MeasurementSeries{}, query.ErrMeasurementServiceUnavailable
	}

	resp, err := c.client.GetMeasurements(ctx, &measurementsv1.GetMeasurementsRequest{
		AssetId: assetID,
		From:    timestamppb.New(from.UTC()),
		To:      timestamppb.New(to.UTC()),
	})
	if err != nil {
		return query.MeasurementSeries{}, mapGRPCError(err)
	}

	return toMeasurementSeries(resp)
}

func mapGRPCError(err error) error {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded:
		return fmt.Errorf("%w: %v", query.ErrMeasurementServiceUnavailable, err)
	default:
		return fmt.Errorf("measurement service get measurements: %w", err)
	}
}

func toMeasurementSeries(resp *measurementsv1.GetMeasurementsResponse) (query.MeasurementSeries, error) {
	if resp == nil {
		return query.MeasurementSeries{}, errors.New("measurement service returned nil response")
	}

	points := make([]query.MeasurementPoint, 0, len(resp.GetPoints()))
	for _, point := range resp.GetPoints() {
		timestamp, err := timestampToTime(point.GetTimestamp())
		if err != nil {
			return query.MeasurementSeries{}, err
		}

		points = append(points, query.MeasurementPoint{
			Timestamp:   timestamp,
			Setpoint:    point.GetSetpoint(),
			ActivePower: point.GetActivePower(),
		})
	}

	return query.MeasurementSeries{
		AssetID: resp.GetAssetId(),
		Points:  points,
	}, nil
}

func timestampToTime(ts *timestamppb.Timestamp) (time.Time, error) {
	if ts == nil {
		return time.Time{}, errors.New("measurement point timestamp is required")
	}

	if err := ts.CheckValid(); err != nil {
		return time.Time{}, fmt.Errorf("invalid measurement point timestamp: %w", err)
	}

	return ts.AsTime().UTC(), nil
}
