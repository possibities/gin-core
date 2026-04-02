package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-boilerplate/pkg/config"
	pkglogger "github.com/possibities/gin-boilerplate/pkg/logger"
	pkgtracing "github.com/possibities/gin-boilerplate/pkg/tracing"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func TestRequestIDNormalizesInboundUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		traceID, _ := c.Get(TraceIDKey)
		if traceID != "550e8400e29b41d4a716446655440000" {
			t.Fatalf("expected normalized trace id, got %v", traceID)
		}
		if pkglogger.TraceIDFromContext(c.Request.Context()) != "550e8400e29b41d4a716446655440000" {
			t.Fatal("expected request context to carry normalized trace id")
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Trace-Id", "550e8400-e29b-41d4-a716-446655440000")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if got := recorder.Header().Get("X-Trace-Id"); got != "550e8400e29b41d4a716446655440000" {
		t.Fatalf("expected response header to carry normalized trace id, got %q", got)
	}
}

func TestRequestIDGeneratesValidTraceIDForInvalidInboundValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		traceID, _ := c.Get(TraceIDKey)
		value, ok := traceID.(string)
		if !ok {
			t.Fatalf("expected trace id string, got %T", traceID)
		}
		if _, ok := pkgtracing.NormalizeTraceID(value); !ok {
			t.Fatalf("expected generated valid trace id, got %q", value)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Trace-Id", "external-trace")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if got := recorder.Header().Get("X-Trace-Id"); got == "external-trace" || got == "" {
		t.Fatalf("expected invalid inbound trace id to be replaced, got %q", got)
	}
}

func TestTracingUsesRequestTraceIDAsSpanTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider, cleanup, err := pkgtracing.New(&config.Config{
		App: config.AppConfig{Name: "gin-boilerplate", Env: "test"},
		Tracing: config.TracingConfig{
			Enabled:     true,
			SampleRatio: 1,
			TimeoutSec:  1,
		},
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cleanup()

	var spanTraceID string
	router := gin.New()
	router.Use(RequestID(), Tracing(provider))
	router.GET("/", func(c *gin.Context) {
		spanTraceID = trace.SpanFromContext(c.Request.Context()).SpanContext().TraceID().String()
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Trace-Id", "550e8400-e29b-41d4-a716-446655440000")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if spanTraceID != "550e8400e29b41d4a716446655440000" {
		t.Fatalf("expected span trace id to match request trace id, got %q", spanTraceID)
	}
}
