package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-core/pkg/response"
)

type stubChecker struct {
	name string
	err  error
}

func (s *stubChecker) Name() string { return s.name }
func (s *stubChecker) Check() error { return s.err }

func TestHealthzReturnsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHealthHandler(nil)
	router := gin.New()
	router.GET("/healthz", h.Healthz)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body response.Body
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 {
		t.Fatalf("expected code 0, got %d", body.Code)
	}
}

func TestHealthzReturns503WhenShuttingDown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHealthHandler(nil)
	h.SetShuttingDown(true)
	router := gin.New()
	router.GET("/healthz", h.Healthz)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestReadyzReturnsOKWhenAllDependenciesHealthy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHealthHandler([]DependencyChecker{
		&stubChecker{name: "db", err: nil},
		&stubChecker{name: "redis", err: nil},
	})
	router := gin.New()
	router.GET("/readyz", h.Readyz)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyzReturns503WhenDependencyFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHealthHandler([]DependencyChecker{
		&stubChecker{name: "db", err: nil},
		&stubChecker{name: "redis", err: errors.New("connection refused")},
	})
	router := gin.New()
	router.GET("/readyz", h.Readyz)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	var body response.Body
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 10007 {
		t.Fatalf("expected dependency not ready code 10007, got %d", body.Code)
	}
}

func TestReadyzReturns503WhenShuttingDown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHealthHandler(nil)
	h.SetShuttingDown(true)
	router := gin.New()
	router.GET("/readyz", h.Readyz)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
