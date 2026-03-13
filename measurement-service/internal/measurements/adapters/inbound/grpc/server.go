package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	getmeasurements "stellar/internal/measurements/application"
	"time"

	"github.com/google/uuid"
	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	measurementsv1 "stellar/api/proto"
)

var _ measurementsv1.MeasurementServiceServer = (*Server)(nil)

type TransportConfig struct {
	ConnectionTimeout   time.Duration
	MaxRecvMsgSizeBytes int
	MaxSendMsgSizeBytes int
	KeepaliveTime       time.Duration
	KeepaliveTimeout    time.Duration
	KeepaliveMinTime    time.Duration
}

const (
	RequestIDMetadataKey     = "x-request-id"
	CorrelationIDMetadataKey = "x-correlation-id"
)

type requestIDContextKey struct{}

type Server struct {
	measurementsv1.UnimplementedMeasurementServiceServer

	queryHandler func() getmeasurements.UseCase
}

func NewServer(queryHandler getmeasurements.UseCase) *Server {
	return &Server{
		queryHandler: func() getmeasurements.UseCase {
			return queryHandler
		},
	}
}

func NewTransport(logger *slog.Logger, queryHandler getmeasurements.UseCase, cfg TransportConfig) *grpcpkg.Server {
	server := grpcpkg.NewServer(
		grpcpkg.ConnectionTimeout(cfg.ConnectionTimeout),
		grpcpkg.MaxRecvMsgSize(cfg.MaxRecvMsgSizeBytes),
		grpcpkg.MaxSendMsgSize(cfg.MaxSendMsgSizeBytes),
		grpcpkg.KeepaliveParams(keepalive.ServerParameters{
			Time:    cfg.KeepaliveTime,
			Timeout: cfg.KeepaliveTimeout,
		}),
		grpcpkg.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime: cfg.KeepaliveMinTime,
		}),
		grpcpkg.ChainUnaryInterceptor(
			requestIDUnaryInterceptor(),
			loggingUnaryInterceptor(logger),
			recoveryUnaryInterceptor(logger),
		),
	)

	measurementsv1.RegisterMeasurementServiceServer(server, NewServer(queryHandler))

	return server
}

func (s *Server) GetMeasurements(ctx context.Context, req *measurementsv1.GetMeasurementsRequest) (*measurementsv1.GetMeasurementsResponse, error) {
	if s.queryHandler == nil {
		return nil, status.Error(codes.Unavailable, "measurements read model unavailable")
	}

	queryHandler := s.queryHandler()

	qry, err := toQuery(req)
	if err != nil {
		return nil, err
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, mapQueryError(err)
	}

	return toGetMeasurementsResponse(result), nil
}

func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

func requestIDUnaryInterceptor() grpcpkg.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpcpkg.UnaryServerInfo,
		handler grpcpkg.UnaryHandler,
	) (any, error) {
		requestID := requestIDFromIncomingContext(ctx)
		_ = grpcpkg.SetHeader(ctx, metadata.Pairs(RequestIDMetadataKey, requestID))

		return handler(withRequestID(ctx, requestID), req)
	}
}

func loggingUnaryInterceptor(logger *slog.Logger) grpcpkg.UnaryServerInterceptor {
	logger = fallbackLogger(logger)

	return func(
		ctx context.Context,
		req any,
		info *grpcpkg.UnaryServerInfo,
		handler grpcpkg.UnaryHandler,
	) (resp any, err error) {
		start := time.Now()
		resp, err = handler(ctx, req)

		logger.Info("handled gRPC unary request",
			"method", info.FullMethod,
			"request_id", RequestIDFromContext(ctx),
			"code", status.Code(err).String(),
			"duration", time.Since(start),
		)

		return resp, err
	}
}

func recoveryUnaryInterceptor(logger *slog.Logger) grpcpkg.UnaryServerInterceptor {
	logger = fallbackLogger(logger)

	return func(
		ctx context.Context,
		req any,
		info *grpcpkg.UnaryServerInfo,
		handler grpcpkg.UnaryHandler,
	) (resp any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("recovered panic in gRPC unary handler",
					"method", info.FullMethod,
					"request_id", RequestIDFromContext(ctx),
					"panic", fmt.Sprint(recovered),
					"stack", string(debug.Stack()),
				)
				resp = nil
				err = status.Error(codes.Internal, "internal server error")
			}
		}()

		return handler(ctx, req)
	}
}

func fallbackLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}

	return slog.Default()
}

func requestIDFromIncomingContext(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for _, key := range []string{RequestIDMetadataKey, CorrelationIDMetadataKey} {
			values := md.Get(key)
			if len(values) > 0 && values[0] != "" {
				return values[0]
			}
		}
	}

	return uuid.NewString()
}

func withRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}
