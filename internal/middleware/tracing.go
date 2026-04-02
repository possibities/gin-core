package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	pkgtracing "github.com/possibities/gin-core/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func Tracing(provider *pkgtracing.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if traceID, ok := c.Get(TraceIDKey); ok {
			if value, ok := traceID.(string); ok {
				if tracedCtx, ok := pkgtracing.ContextWithTraceID(ctx, value); ok {
					ctx = tracedCtx
				}
			}
		}

		tracer := provider.Tracer("http.server")
		spanName := c.Request.Method + " " + c.Request.URL.Path
		ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
		c.Request = c.Request.WithContext(ctx)
		c.Next()

		route := c.FullPath()
		if route != "" {
			span.SetName(c.Request.Method + " " + route)
			span.SetAttributes(attribute.String("http.route", route))
		}
		span.SetAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("url.path", c.Request.URL.Path),
			attribute.Int("http.status_code", c.Writer.Status()),
		)

		status := c.Writer.Status()
		if len(c.Errors) > 0 {
			span.RecordError(c.Errors.Last())
			span.SetStatus(codes.Error, c.Errors.String())
		} else if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		} else {
			span.SetStatus(codes.Ok, http.StatusText(status))
		}
		span.End()
	}
}
