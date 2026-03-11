package ports

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type GRPCTransportSuite struct {
	suite.Suite
	logger *slog.Logger
	logs   *bytes.Buffer
}

func TestGRPCTransportSuite(t *testing.T) {
	suite.Run(t, new(GRPCTransportSuite))
}

func (s *GRPCTransportSuite) SetupTest() {
	s.logs = &bytes.Buffer{}
	s.logger = slog.New(slog.NewJSONHandler(io.MultiWriter(io.Discard, s.logs), nil))
}

func (s *GRPCTransportSuite) TestRequestIDUnaryInterceptorPropagatesIncomingRequestID() {
	stream := &fakeServerTransportStream{}
	ctx := grpc.NewContextWithServerTransportStream(
		metadata.NewIncomingContext(context.Background(), metadata.Pairs(RequestIDMetadataKey, "req-123")),
		stream,
	)

	interceptor := requestIDUnaryInterceptor()

	resp, err := interceptor(
		ctx,
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/measurements.v1.MeasurementService/GetMeasurements"},
		func(ctx context.Context, req any) (any, error) {
			s.Equal("req-123", RequestIDFromContext(ctx))
			return "ok", nil
		},
	)

	s.Require().NoError(err)
	s.Equal("ok", resp)
	s.Equal("req-123", firstMetadataValue(stream.header, RequestIDMetadataKey))
}

func (s *GRPCTransportSuite) TestRequestIDUnaryInterceptorFallsBackToCorrelationID() {
	stream := &fakeServerTransportStream{}
	ctx := grpc.NewContextWithServerTransportStream(
		metadata.NewIncomingContext(context.Background(), metadata.Pairs(CorrelationIDMetadataKey, "corr-123")),
		stream,
	)

	interceptor := requestIDUnaryInterceptor()

	_, err := interceptor(
		ctx,
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/measurements.v1.MeasurementService/GetMeasurements"},
		func(ctx context.Context, req any) (any, error) {
			s.Equal("corr-123", RequestIDFromContext(ctx))
			return nil, nil
		},
	)

	s.Require().NoError(err)
	s.Equal("corr-123", firstMetadataValue(stream.header, RequestIDMetadataKey))
}

func (s *GRPCTransportSuite) TestRequestIDUnaryInterceptorGeneratesRequestIDWhenMissing() {
	stream := &fakeServerTransportStream{}
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)

	interceptor := requestIDUnaryInterceptor()

	var requestID string
	_, err := interceptor(
		ctx,
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/measurements.v1.MeasurementService/GetMeasurements"},
		func(ctx context.Context, req any) (any, error) {
			requestID = RequestIDFromContext(ctx)
			return nil, nil
		},
	)

	s.Require().NoError(err)
	s.NotEmpty(requestID)
	s.Equal(requestID, firstMetadataValue(stream.header, RequestIDMetadataKey))
}

func (s *GRPCTransportSuite) TestLoggingUnaryInterceptorIncludesRequestID() {
	ctx := withRequestID(context.Background(), "req-456")
	interceptor := loggingUnaryInterceptor(s.logger)

	_, err := interceptor(
		ctx,
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/measurements.v1.MeasurementService/GetMeasurements"},
		func(context.Context, any) (any, error) {
			return nil, nil
		},
	)

	s.Require().NoError(err)
	s.Contains(s.logs.String(), "req-456")
}

func (s *GRPCTransportSuite) TestRecoveryUnaryInterceptorConvertsPanicsToInternalError() {
	interceptor := recoveryUnaryInterceptor(s.logger)

	resp, err := interceptor(
		withRequestID(context.Background(), "req-panic"),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/measurements.v1.MeasurementService/GetMeasurements"},
		func(context.Context, any) (any, error) {
			panic("boom")
		},
	)

	s.Nil(resp)
	s.Equal(codes.Internal, status.Code(err))
	s.Contains(s.logs.String(), "req-panic")
}

func (s *GRPCTransportSuite) TestRecoveryStreamInterceptorConvertsPanicsToInternalError() {
	interceptor := recoveryStreamInterceptor(s.logger)

	err := interceptor(
		nil,
		&fakeServerStream{ctx: withRequestID(context.Background(), "stream-panic")},
		&grpc.StreamServerInfo{FullMethod: "/measurements.v1.MeasurementService/StreamMeasurements"},
		func(any, grpc.ServerStream) error {
			panic("boom")
		},
	)

	s.Equal(codes.Internal, status.Code(err))
	s.Contains(s.logs.String(), "stream-panic")
}

type fakeServerTransportStream struct {
	header metadata.MD
}

func (s *fakeServerTransportStream) Method() string {
	return "/measurements.v1.MeasurementService/GetMeasurements"
}

func (s *fakeServerTransportStream) SetHeader(md metadata.MD) error {
	s.header = metadata.Join(s.header, md)
	return nil
}

func (s *fakeServerTransportStream) SendHeader(md metadata.MD) error {
	return s.SetHeader(md)
}

func (s *fakeServerTransportStream) SetTrailer(metadata.MD) error {
	return nil
}

type fakeServerStream struct {
	grpc.ServerStream
	ctx    context.Context
	header metadata.MD
}

func (s *fakeServerStream) Context() context.Context {
	return s.ctx
}

func (s *fakeServerStream) SetHeader(md metadata.MD) error {
	s.header = metadata.Join(s.header, md)
	return nil
}

func (s *fakeServerStream) SendHeader(md metadata.MD) error {
	return s.SetHeader(md)
}

func (s *fakeServerStream) SetTrailer(metadata.MD) {}

func firstMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
