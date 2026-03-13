//go:build integration

package grpc

import (
	"context"
	"net"
	getmeasurements "stellar/internal/measurements/application"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	measurementsv1 "stellar/api/proto"
	"stellar/internal/measurements/adapters/outbound/influxdb"
	"stellar/internal/testsupport"
)

type GRPCIntegrationSuite struct {
	suite.Suite
}

func TestGRPCIntegrationSuite(t *testing.T) {
	suite.Run(t, new(GRPCIntegrationSuite))
}

func (s *GRPCIntegrationSuite) TestGetMeasurements() {
	influx := testsupport.StartInfluxDB(s.T())

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	testsupport.SeedMeasurements(s.T(), influx.Client, influx.Org, influx.Bucket, []testsupport.MeasurementSeed{
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
	useCase, err := getmeasurements.NewUseCase(readModel)
	s.Require().NoError(err)

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	measurementsv1.RegisterMeasurementServiceServer(server, NewServer(useCase))
	s.T().Cleanup(server.Stop)

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
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		s.NoError(conn.Close())
	})

	client := measurementsv1.NewMeasurementServiceClient(conn)

	requestCtx, requestCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer requestCancel()

	resp, err := client.GetMeasurements(requestCtx, &measurementsv1.GetMeasurementsRequest{
		AssetId: "asset-1",
		From:    timestamppb.New(base),
		To:      timestamppb.New(base.Add(2 * time.Second)),
	})
	s.Require().NoError(err)

	s.Equal("asset-1", resp.GetAssetId())
	s.Len(resp.GetPoints(), 2)
	s.True(resp.GetPoints()[0].GetTimestamp().AsTime().Equal(base))
	s.Equal(20.0, resp.GetPoints()[1].GetSetpoint())
	s.Equal(19.0, resp.GetPoints()[1].GetActivePower())

	_, err = client.GetMeasurements(requestCtx, &measurementsv1.GetMeasurementsRequest{
		AssetId: "asset-1",
		To:      timestamppb.New(base),
	})
	s.Equal(codes.InvalidArgument, status.Code(err))
}
