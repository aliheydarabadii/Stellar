package ports

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
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
		grpc.ChainUnaryInterceptor(recoveryUnaryInterceptor(logger)),
		grpc.ChainStreamInterceptor(recoveryStreamInterceptor(logger)),
	)

	measurementsv1.RegisterMeasurementServiceServer(server, NewGRPCServer(application))

	return server
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
