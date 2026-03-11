// Package ports contains the inbound delivery adapters for the measurements service.
package ports

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	measurementsv1 "stellar/api/proto"
	"stellar/internal/measurements/app"
	"stellar/internal/measurements/app/query"
)

type GRPCServer struct {
	measurementsv1.UnimplementedMeasurementServiceServer

	app app.Application
}

func NewGRPCServer(application app.Application) *GRPCServer {
	return &GRPCServer{app: application}
}

func (s *GRPCServer) GetMeasurements(ctx context.Context, req *measurementsv1.GetMeasurementsRequest) (*measurementsv1.GetMeasurementsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	from, err := timestampToTime(req.GetFrom(), "from")
	if err != nil {
		return nil, err
	}

	to, err := timestampToTime(req.GetTo(), "to")
	if err != nil {
		return nil, err
	}

	result, err := s.app.Queries.GetMeasurements.Handle(ctx, query.GetMeasurements{
		AssetID: req.GetAssetId(),
		From:    from,
		To:      to,
	})
	if err != nil {
		return nil, mapQueryError(err)
	}

	return toGetMeasurementsResponse(result), nil
}

func timestampToTime(ts *timestamppb.Timestamp, field string) (time.Time, error) {
	if ts == nil {
		return time.Time{}, status.Errorf(codes.InvalidArgument, "%s is required", field)
	}

	if err := ts.CheckValid(); err != nil {
		return time.Time{}, status.Errorf(codes.InvalidArgument, "invalid %s timestamp: %v", field, err)
	}

	return ts.AsTime().UTC(), nil
}

func mapQueryError(err error) error {
	switch {
	case errors.Is(err, query.ErrAssetIDRequired),
		errors.Is(err, query.ErrInvalidTimeRange),
		errors.Is(err, query.ErrQueryRangeTooLarge),
		errors.Is(err, query.ErrTimestampZero):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, query.ErrReadModelUnavailable):
		return status.Error(codes.Unavailable, "measurements read model unavailable")
	default:
		return status.Error(codes.Internal, "get measurements failed")
	}
}

func toGetMeasurementsResponse(series query.MeasurementSeries) *measurementsv1.GetMeasurementsResponse {
	points := make([]*measurementsv1.MeasurementPoint, 0, len(series.Points))
	for _, point := range series.Points {
		points = append(points, &measurementsv1.MeasurementPoint{
			Timestamp:   timestamppb.New(point.Timestamp),
			Setpoint:    point.Setpoint,
			ActivePower: point.ActivePower,
		})
	}

	return &measurementsv1.GetMeasurementsResponse{
		AssetId: series.AssetID,
		Points:  points,
	}
}
