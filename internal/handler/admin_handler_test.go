package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-core/internal/middleware"
	"github.com/possibities/gin-core/internal/model"
	"github.com/possibities/gin-core/internal/service"
	"github.com/possibities/gin-core/pkg/response"
)

type stubAuditLogRepo struct {
	createErr error
	captured  *model.AuditLog
}

func (s *stubAuditLogRepo) Create(_ context.Context, log *model.AuditLog) error {
	s.captured = log
	return s.createErr
}

func TestAdminCurrentReturnsSessionWithAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubAuditLogRepo{}
	h := NewAdminHandler(service.NewAuditService(repo))

	router := gin.New()
	router.GET("/admin/session", func(c *gin.Context) {
		c.Set(middleware.UserIDKey, uint(1))
		c.Set(middleware.RoleKey, "admin")
		c.Set(middleware.TenantIDKey, "tenant-a")
		c.Set(middleware.TraceIDKey, "trace-123")
		h.Current(c)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/session", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body response.Body
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	data := body.Data.(map[string]any)
	if data["scope"] != "admin" {
		t.Fatalf("expected scope admin, got %v", data["scope"])
	}

	if repo.captured == nil {
		t.Fatal("expected audit log to be recorded")
	}
	if repo.captured.Action != "admin.session.view" {
		t.Fatalf("expected action admin.session.view, got %s", repo.captured.Action)
	}
	if repo.captured.TraceID != "trace-123" {
		t.Fatalf("expected trace_id trace-123, got %s", repo.captured.TraceID)
	}
}

func TestAdminCurrentReturnsErrorWhenAuditFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubAuditLogRepo{createErr: errors.New("db write failed")}
	h := NewAdminHandler(service.NewAuditService(repo))

	router := gin.New()
	router.GET("/admin/session", func(c *gin.Context) {
		c.Set(middleware.UserIDKey, uint(1))
		c.Set(middleware.RoleKey, "admin")
		h.Current(c)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/session", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
