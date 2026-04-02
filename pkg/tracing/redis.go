package tracing

import (
	"context"
	"net"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type redisHook struct {
	provider *Provider
}

func NewRedisHook(provider *Provider) redis.Hook {
	return redisHook{provider: provider}
}

func (h redisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h redisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		tracer := h.provider.Tracer("redis")
		ctx, span := tracer.Start(ctx, "redis."+cmd.Name(), trace.WithSpanKind(trace.SpanKindClient))
		span.SetAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", cmd.Name()),
		)
		err := next(ctx, cmd)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, cmd.Name())
		}
		span.End()
		return err
	}
}

func (h redisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		tracer := h.provider.Tracer("redis")
		ctx, span := tracer.Start(ctx, "redis.pipeline", trace.WithSpanKind(trace.SpanKindClient))
		span.SetAttributes(
			attribute.String("db.system", "redis"),
			attribute.Int("redis.pipeline.length", len(cmds)),
		)
		err := next(ctx, cmds)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "pipeline")
		}
		span.End()
		return err
	}
}
