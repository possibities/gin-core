package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-core/internal/middleware"
	"github.com/possibities/gin-core/pkg/response"
)

func TestSessionCurrentReturnsContextValues(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewSessionHandler()
	router := gin.New()
	router.GET("/session", func(c *gin.Context) {
		c.Set(middleware.UserIDKey, uint(42))
		c.Set(middleware.RoleKey, "admin")
		c.Set(middleware.TenantIDKey, "tenant-a")
		h.Current(c)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/session", nil))

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

	data, ok := body.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data map, got %T", body.Data)
	}
	if uint(data["user_id"].(float64)) != 42 {
		t.Fatalf("expected user_id 42, got %v", data["user_id"])
	}
	if data["role"] != "admin" {
		t.Fatalf("expected role admin, got %v", data["role"])
	}
	if data["tenant_id"] != "tenant-a" {
		t.Fatalf("expected tenant_id tenant-a, got %v", data["tenant_id"])
	}
}

func TestSessionCurrentHandlesMissingContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewSessionHandler()
	router := gin.New()
	router.GET("/session", h.Current)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/session", nil))

	// Session handler returns 200 with nil values when context is missing,
	// because auth middleware is responsible for rejecting unauthenticated requests.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
