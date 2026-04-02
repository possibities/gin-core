package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/possibities/gin-boilerplate/pkg/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Provider struct {
	provider *sdktrace.TracerProvider
}

type contextKey string

const preferredTraceIDContextKey contextKey = "preferred_trace_id"

type idGenerator struct{}

func New(cfg *config.Config, logger *zap.Logger) (*Provider, func(), error) {
	resource, err := sdkresource.Merge(
		sdkresource.Default(),
		sdkresource.NewWithAttributes(
			"",
			attribute.String("service.name", cfg.App.Name),
			attribute.String("deployment.environment.name", cfg.App.Env),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("build tracing resource: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Tracing.SampleRatio))
	if !cfg.Tracing.Enabled {
		sampler = sdktrace.NeverSample()
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource),
		sdktrace.WithSampler(sampler),
		sdktrace.WithIDGenerator(idGenerator{}),
	}

	if cfg.Tracing.Enabled && strings.TrimSpace(cfg.Tracing.Endpoint) != "" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Tracing.TimeoutSec)*time.Second)
		defer cancel()

		exporterOptions := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(strings.TrimSpace(cfg.Tracing.Endpoint)),
		}
		if cfg.Tracing.Insecure {
			exporterOptions = append(exporterOptions, otlptracehttp.WithInsecure())
		}

		exporter, err := otlptracehttp.New(ctx, exporterOptions...)
		if err != nil {
			return nil, nil, fmt.Errorf("create otlp trace exporter: %w", err)
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	} else if cfg.Tracing.Enabled && logger != nil {
		logger.Warn("tracing enabled without exporter endpoint; spans will remain local")
	}

	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Tracing.TimeoutSec)*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	}

	return &Provider{provider: provider}, cleanup, nil
}

func (p *Provider) Tracer(name string) trace.Tracer {
	if p == nil || p.provider == nil {
		return otel.Tracer(name)
	}
	return p.provider.Tracer(name)
}

func NewTraceID() string {
	id := uuid.New()
	return hex.EncodeToString(id[:])
}

func NormalizeTraceID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}

	if tid, err := trace.TraceIDFromHex(value); err == nil && tid.IsValid() {
		return tid.String(), true
	}

	parsed, err := uuid.Parse(value)
	if err != nil {
		return "", false
	}
	return hex.EncodeToString(parsed[:]), true
}

func ContextWithTraceID(ctx context.Context, traceID string) (context.Context, bool) {
	normalized, ok := NormalizeTraceID(traceID)
	if !ok {
		return ctx, false
	}
	return context.WithValue(ctx, preferredTraceIDContextKey, normalized), true
}

func (idGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	if traceID, ok := ctx.Value(preferredTraceIDContextKey).(string); ok {
		if parsed, err := trace.TraceIDFromHex(traceID); err == nil && parsed.IsValid() {
			return parsed, newSpanID()
		}
	}
	return newTraceID(), newSpanID()
}

func (idGenerator) NewSpanID(context.Context, trace.TraceID) trace.SpanID {
	return newSpanID()
}

func newTraceID() trace.TraceID {
	var traceID trace.TraceID
	if _, err := rand.Read(traceID[:]); err != nil || !traceID.IsValid() {
		generated, _ := trace.TraceIDFromHex(NewTraceID())
		return generated
	}
	return traceID
}

func newSpanID() trace.SpanID {
	var spanID trace.SpanID
	if _, err := rand.Read(spanID[:]); err != nil || !spanID.IsValid() {
		for !spanID.IsValid() {
			_, _ = rand.Read(spanID[:])
		}
	}
	return spanID
}
