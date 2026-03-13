package grpc

import (
	"errors"
	"stellar/internal/measurements"
	"stellar/internal/measurements/application"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	measurementsv1 "stellar/api/proto"
)

func toQuery(req *measurementsv1.GetMeasurementsRequest) (application.Query, error) {
	if req == nil {
		return application.Query{}, status.Error(codes.InvalidArgument, "request is required")
	}

	from, err := timestampToTime(req.GetFrom(), "from")
	if err != nil {
		return application.Query{}, err
	}

	to, err := timestampToTime(req.GetTo(), "to")
	if err != nil {
		return application.Query{}, err
	}

	return application.Query{
		AssetID: req.GetAssetId(),
		From:    from,
		To:      to,
	}, nil
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
	case errors.Is(err, application.ErrAssetIDRequired),
		errors.Is(err, application.ErrInvalidTimeRange),
		errors.Is(err, application.ErrQueryRangeTooLarge),
		errors.Is(err, application.ErrTimestampZero):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, application.ErrReadModelUnavailable):
		return status.Error(codes.Unavailable, "measurements read model unavailable")
	default:
		return status.Error(codes.Internal, "get measurements failed")
	}
}

func toGetMeasurementsResponse(series measurements.MeasurementSeries) *measurementsv1.GetMeasurementsResponse {
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
