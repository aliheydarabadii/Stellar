package ports

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GRPCTransportSuite struct {
	suite.Suite
	logger *slog.Logger
}

func TestGRPCTransportSuite(t *testing.T) {
	suite.Run(t, new(GRPCTransportSuite))
}

func (s *GRPCTransportSuite) SetupTest() {
	s.logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func (s *GRPCTransportSuite) TestRecoveryUnaryInterceptorConvertsPanicsToInternalError() {
	interceptor := recoveryUnaryInterceptor(s.logger)

	resp, err := interceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/measurements.v1.MeasurementService/GetMeasurements"},
		func(context.Context, any) (any, error) {
			panic("boom")
		},
	)

	s.Nil(resp)
	s.Equal(codes.Internal, status.Code(err))
}

func (s *GRPCTransportSuite) TestRecoveryStreamInterceptorConvertsPanicsToInternalError() {
	interceptor := recoveryStreamInterceptor(s.logger)

	err := interceptor(
		nil,
		nil,
		&grpc.StreamServerInfo{FullMethod: "/measurements.v1.MeasurementService/StreamMeasurements"},
		func(any, grpc.ServerStream) error {
			panic("boom")
		},
	)

	s.Equal(codes.Internal, status.Code(err))
}
