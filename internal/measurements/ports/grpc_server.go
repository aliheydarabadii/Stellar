package ports

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	measurementsv1 "stellar/api/proto"
	"stellar/internal/measurements/app"
)

type GRPCTransportConfig struct {
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

func NewGRPCTransport(logger *slog.Logger, application app.Application, cfg GRPCTransportConfig) *grpc.Server {
	server := grpc.NewServer(
		grpc.ConnectionTimeout(cfg.ConnectionTimeout),
		grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSizeBytes),
		grpc.MaxSendMsgSize(cfg.MaxSendMsgSizeBytes),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    cfg.KeepaliveTime,
			Timeout: cfg.KeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime: cfg.KeepaliveMinTime,
		}),
		grpc.ChainUnaryInterceptor(
			requestIDUnaryInterceptor(),
			loggingUnaryInterceptor(logger),
			recoveryUnaryInterceptor(logger),
		),
		grpc.ChainStreamInterceptor(
			requestIDStreamInterceptor(),
			loggingStreamInterceptor(logger),
			recoveryStreamInterceptor(logger),
		),
	)

	measurementsv1.RegisterMeasurementServiceServer(server, NewGRPCServer(application))

	return server
}

func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

func requestIDUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		requestID := requestIDFromIncomingContext(ctx)
		_ = grpc.SetHeader(ctx, metadata.Pairs(RequestIDMetadataKey, requestID))

		return handler(withRequestID(ctx, requestID), req)
	}
}

func requestIDStreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		requestID := requestIDFromIncomingContext(stream.Context())
		_ = stream.SetHeader(metadata.Pairs(RequestIDMetadataKey, requestID))

		return handler(srv, &requestIDServerStream{
			ServerStream: stream,
			ctx:          withRequestID(stream.Context(), requestID),
		})
	}
}

func loggingUnaryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	logger = fallbackLogger(logger)

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
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

func loggingStreamInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	logger = fallbackLogger(logger)

	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		start := time.Now()
		err = handler(srv, stream)

		logger.Info("handled gRPC stream request",
			"method", info.FullMethod,
			"request_id", RequestIDFromContext(stream.Context()),
			"code", status.Code(err).String(),
			"duration", time.Since(start),
		)

		return err
	}
}

func recoveryUnaryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	logger = fallbackLogger(logger)

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
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

func recoveryStreamInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	logger = fallbackLogger(logger)

	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("recovered panic in gRPC stream handler",
					"method", info.FullMethod,
					"request_id", RequestIDFromContext(stream.Context()),
					"panic", fmt.Sprint(recovered),
					"stack", string(debug.Stack()),
				)
				err = status.Error(codes.Internal, "internal server error")
			}
		}()

		return handler(srv, stream)
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

type requestIDServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *requestIDServerStream) Context() context.Context {
	return s.ctx
}
