package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"api_gateway/internal/measurements/domain"
	"api_gateway/internal/platform/requestctx"
	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	measurementsv1 "stellar/api/proto"
)

type Client struct {
	conn   *grpcpkg.ClientConn
	client measurementsv1.MeasurementServiceClient
}

const readinessProbeAssetID = "gateway-readiness-probe"

func Dial(ctx context.Context, address string) (*Client, error) {
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

	return &Client{
		conn:   conn,
		client: measurementsv1.NewMeasurementServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func (c *Client) Ready(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("measurement service client is not initialized")
	}

	now := time.Now().UTC().Truncate(time.Second)
	_, err := c.client.GetMeasurements(ctx, &measurementsv1.GetMeasurementsRequest{
		AssetId: readinessProbeAssetID,
		From:    timestamppb.New(now),
		To:      timestamppb.New(now),
	})
	if err != nil {
		return fmt.Errorf("probe measurement service readiness: %w", err)
	}

	return nil
}

func (c *Client) GetMeasurements(ctx context.Context, assetID string, from, to time.Time) (domain.MeasurementSeries, error) {
	if c == nil || c.client == nil {
		return domain.MeasurementSeries{}, serviceUnavailableError{cause: errors.New("measurement service client is not initialized")}
	}

	ctx = withOutgoingRequestMetadata(ctx)

	resp, err := c.client.GetMeasurements(ctx, &measurementsv1.GetMeasurementsRequest{
		AssetId: assetID,
		From:    timestamppb.New(from.UTC()),
		To:      timestamppb.New(to.UTC()),
	})
	if err != nil {
		return domain.MeasurementSeries{}, mapGRPCError(err)
	}

	return toMeasurementSeries(resp)
}

func mapGRPCError(err error) error {
	statusErr := status.Convert(err)

	switch statusErr.Code() {
	case codes.InvalidArgument:
		return newDownstreamInvalidRequestError(statusErr.Message())
	case codes.Unavailable, codes.DeadlineExceeded:
		return serviceUnavailableError{cause: err}
	default:
		return fmt.Errorf("measurement service get measurements: %w", err)
	}
}

func toMeasurementSeries(resp *measurementsv1.GetMeasurementsResponse) (domain.MeasurementSeries, error) {
	if resp == nil {
		return domain.MeasurementSeries{}, errors.New("measurement service returned nil response")
	}

	points := make([]domain.MeasurementPoint, 0, len(resp.GetPoints()))
	for _, point := range resp.GetPoints() {
		timestamp, err := timestampToTime(point.GetTimestamp())
		if err != nil {
			return domain.MeasurementSeries{}, err
		}

		points = append(points, domain.MeasurementPoint{
			Timestamp:   timestamp,
			Setpoint:    point.GetSetpoint(),
			ActivePower: point.GetActivePower(),
		})
	}

	return domain.MeasurementSeries{
		AssetID: resp.GetAssetId(),
		Points:  points,
	}, nil
}

func withOutgoingRequestMetadata(ctx context.Context) context.Context {
	requestID := requestctx.RequestIDFromContext(ctx)
	correlationID := requestctx.CorrelationIDFromContext(ctx)

	if requestID == "" && correlationID == "" {
		return ctx
	}

	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	if requestID != "" {
		md.Set(requestctx.RequestIDHeader, requestID)
	}
	if correlationID != "" {
		md.Set(requestctx.CorrelationIDHeader, correlationID)
	}

	return metadata.NewOutgoingContext(ctx, md)
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

type serviceUnavailableError struct {
	cause error
}

func (e serviceUnavailableError) Error() string {
	if e.cause == nil {
		return "measurement service unavailable"
	}

	return fmt.Sprintf("measurement service unavailable: %v", e.cause)
}

func (e serviceUnavailableError) Unwrap() error {
	return e.cause
}

func (e serviceUnavailableError) MeasurementServiceUnavailable() bool {
	return true
}

type downstreamInvalidRequestError struct {
	message string
}

func newDownstreamInvalidRequestError(message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "measurement service rejected request"
	}

	return downstreamInvalidRequestError{message: message}
}

func (e downstreamInvalidRequestError) Error() string {
	return e.message
}

func (e downstreamInvalidRequestError) DownstreamInvalidRequestMessage() string {
	return e.message
}
