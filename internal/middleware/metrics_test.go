package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-boilerplate/pkg/metrics"
)

type fakeWorkerQueue struct{}

func (fakeWorkerQueue) QueueDepth() int {
	return 0
}

func TestMetricsRecordsRoutePatternAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	registry := metrics.New(fakeWorkerQueue{})
	router := gin.New()
	router.Use(Metrics(registry))
	router.GET("/users/:id", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	router.ServeHTTP(recorder, req)

	metricsRecorder := httptest.NewRecorder()
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	registry.Handler().ServeHTTP(metricsRecorder, metricsReq)

	body := metricsRecorder.Body.String()
	if !strings.Contains(body, "http_requests_total{method=\"GET\",path=\"/users/:id\",status=\"204\"} 1") {
		t.Fatalf("expected metrics to use route pattern label, got %s", body)
	}
}

func TestMetricsRecordsAbortedRequestsWhenPlacedBeforeAbortMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	registry := metrics.New(fakeWorkerQueue{})
	router := gin.New()
	router.Use(
		Metrics(registry),
		func(c *gin.Context) {
			c.AbortWithStatus(http.StatusTooManyRequests)
		},
	)
	router.GET("/limited", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	router.ServeHTTP(recorder, req)

	metricsRecorder := httptest.NewRecorder()
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	registry.Handler().ServeHTTP(metricsRecorder, metricsReq)

	body := metricsRecorder.Body.String()
	if !strings.Contains(body, "http_requests_total{method=\"GET\",path=\"/limited\",status=\"429\"} 1") {
		t.Fatalf("expected aborted request to be recorded, got %s", body)
	}
}
