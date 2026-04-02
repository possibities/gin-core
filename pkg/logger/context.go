package pkglogger

import (
	"context"

	"go.uber.org/zap"
)

type contextKey string

const (
	loggerContextKey  contextKey = "logger"
	traceIDContextKey contextKey = "trace_id"
)

func WithRequestContext(ctx context.Context, logger *zap.Logger, traceID string) context.Context {
	ctx = context.WithValue(ctx, loggerContextKey, logger)
	return context.WithValue(ctx, traceIDContextKey, traceID)
}

func FromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return zap.L()
	}
	if logger, ok := ctx.Value(loggerContextKey).(*zap.Logger); ok && logger != nil {
		return logger
	}
	return zap.L()
}

func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if traceID, ok := ctx.Value(traceIDContextKey).(string); ok {
		return traceID
	}
	return ""
}

func WithTraceID(base *zap.Logger, traceID string) *zap.Logger {
	if base == nil {
		base = zap.L()
	}
	if traceID == "" {
		return base
	}
	return base.With(zap.String("trace_id", traceID))
}
