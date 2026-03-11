package requestctx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
)

const (
	RequestIDHeader     = "x-request-id"
	CorrelationIDHeader = "x-correlation-id"
)

type contextKey string

const (
	requestIDContextKey     contextKey = "request_id"
	correlationIDContextKey contextKey = "correlation_id"
)

func Normalize(requestID, correlationID string) (string, string) {
	normalizedRequestID := strings.TrimSpace(requestID)
	normalizedCorrelationID := strings.TrimSpace(correlationID)

	if normalizedRequestID == "" {
		normalizedRequestID = normalizedCorrelationID
	}
	if normalizedRequestID == "" {
		normalizedRequestID = newRequestID()
	}

	return normalizedRequestID, normalizedCorrelationID
}

func WithValues(ctx context.Context, requestID, correlationID string) context.Context {
	ctx = context.WithValue(ctx, requestIDContextKey, requestID)
	ctx = context.WithValue(ctx, correlationIDContextKey, correlationID)
	return ctx
}

func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey).(string)
	return value
}

func CorrelationIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(correlationIDContextKey).(string)
	return value
}

func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "generated-request-id"
	}

	return hex.EncodeToString(buf[:])
}
