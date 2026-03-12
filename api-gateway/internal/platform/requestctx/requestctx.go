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

const (
	CacheStatusHit           = "hit"
	CacheStatusMiss          = "miss"
	CacheStatusBypass        = "bypass"
	CacheStatusNotApplicable = "not_applicable"
)

type contextKey string

const (
	requestIDContextKey     contextKey = "request_id"
	correlationIDContextKey contextKey = "correlation_id"
	requestStateContextKey  contextKey = "request_state"
)

type requestState struct {
	requestID     string
	correlationID string
	cacheStatus   string
}

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
	state := stateFromContext(ctx)
	if state == nil {
		state = &requestState{}
	}

	state.requestID = requestID
	state.correlationID = correlationID

	ctx = context.WithValue(ctx, requestStateContextKey, state)
	ctx = context.WithValue(ctx, requestIDContextKey, requestID)
	ctx = context.WithValue(ctx, correlationIDContextKey, correlationID)

	return ctx
}

func RequestIDFromContext(ctx context.Context) string {
	if state := stateFromContext(ctx); state != nil && state.requestID != "" {
		return state.requestID
	}

	value, _ := ctx.Value(requestIDContextKey).(string)
	return value
}

func CorrelationIDFromContext(ctx context.Context) string {
	if state := stateFromContext(ctx); state != nil && state.correlationID != "" {
		return state.correlationID
	}

	value, _ := ctx.Value(correlationIDContextKey).(string)
	return value
}

func SetCacheStatus(ctx context.Context, cacheStatus string) {
	if state := stateFromContext(ctx); state != nil {
		state.cacheStatus = cacheStatus
	}
}

func CacheStatusFromContext(ctx context.Context) string {
	if state := stateFromContext(ctx); state != nil {
		return state.cacheStatus
	}

	return ""
}

func stateFromContext(ctx context.Context) *requestState {
	state, _ := ctx.Value(requestStateContextKey).(*requestState)
	return state
}

func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "generated-request-id"
	}

	return hex.EncodeToString(buf[:])
}
