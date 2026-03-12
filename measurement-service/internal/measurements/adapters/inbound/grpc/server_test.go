package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	measurementsv1 "stellar/api/proto"
	getmeasurements "stellar/internal/measurements/application/get_measurements"
	"stellar/internal/measurements/domain"
)

type GRPCServerSuite struct {
	suite.Suite
}

func TestGRPCServerSuite(t *testing.T) {
	suite.Run(t, new(GRPCServerSuite))
}

func (s *GRPCServerSuite) TestGetMeasurementsMapsRequestAndResponse() {
	from := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Second)
	readModel := &capturingReadModel{
		points: []domain.MeasurementPoint{
			{
				Timestamp:   from,
				Setpoint:    11,
				ActivePower: 10.5,
			},
		},
	}

	useCase, err := getmeasurements.NewUseCase(readModel)
	s.Require().NoError(err)

	server := NewServer(useCase)

	resp, err := server.GetMeasurements(context.Background(), &measurementsv1.GetMeasurementsRequest{
		AssetId: "asset-1",
		From:    timestamppb.New(from),
		To:      timestamppb.New(to),
	})
	s.Require().NoError(err)

	s.Equal("asset-1", readModel.assetID)
	s.True(readModel.from.Equal(from))
	s.True(readModel.to.Equal(to))
	s.Equal("asset-1", resp.GetAssetId())
	s.Len(resp.GetPoints(), 1)
	s.Equal(11.0, resp.GetPoints()[0].GetSetpoint())
	s.Equal(10.5, resp.GetPoints()[0].GetActivePower())
}

func (s *GRPCServerSuite) TestGetMeasurementsRejectsInvalidInput() {
	now := time.Now().UTC()

	useCase, err := getmeasurements.NewUseCase(&capturingReadModel{})
	s.Require().NoError(err)

	server := NewServer(useCase)

	_, err = server.GetMeasurements(context.Background(), &measurementsv1.GetMeasurementsRequest{
		AssetId: "",
		From:    timestamppb.New(now),
		To:      timestamppb.New(now),
	})

	s.Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
}

func (s *GRPCServerSuite) TestGetMeasurementsMapsReadModelUnavailable() {
	now := time.Now().UTC()

	useCase, err := getmeasurements.NewUseCase(&capturingReadModel{err: getmeasurements.ErrReadModelUnavailable})
	s.Require().NoError(err)

	server := NewServer(useCase)

	_, err = server.GetMeasurements(context.Background(), &measurementsv1.GetMeasurementsRequest{
		AssetId: "asset-1",
		From:    timestamppb.New(now),
		To:      timestamppb.New(now),
	})

	s.Error(err)
	s.Equal(codes.Unavailable, status.Code(err))
}

func (s *GRPCServerSuite) TestGetMeasurementsRejectsInvalidRequestShape() {
	useCase, err := getmeasurements.NewUseCase(&capturingReadModel{})
	s.Require().NoError(err)

	server := NewServer(useCase)

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
		s.Run(tc.name, func() {
			_, err := server.GetMeasurements(context.Background(), tc.req)

			s.Error(err)
			s.Equal(codes.InvalidArgument, status.Code(err))
		})
	}
}

func (s *GRPCServerSuite) TestGetMeasurementsRejectsRangeLargerThanConfiguredLimit() {
	now := time.Now().UTC().Truncate(time.Second)

	useCase, err := getmeasurements.NewUseCaseWithConfig(&capturingReadModel{}, getmeasurements.Config{
		MaxQueryRange: 15 * time.Minute,
	})
	s.Require().NoError(err)

	server := NewServer(useCase)

	_, err = server.GetMeasurements(context.Background(), &measurementsv1.GetMeasurementsRequest{
		AssetId: "asset-1",
		From:    timestamppb.New(now),
		To:      timestamppb.New(now.Add(15*time.Minute + time.Second)),
	})

	s.Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
}

type capturingReadModel struct {
	assetID string
	from    time.Time
	to      time.Time
	points  []domain.MeasurementPoint
	err     error
}

func (r *capturingReadModel) GetMeasurements(_ context.Context, assetID string, from, to time.Time) ([]domain.MeasurementPoint, error) {
	r.assetID = assetID
	r.from = from
	r.to = to

	if r.err != nil {
		return nil, r.err
	}

	return r.points, nil
}
