package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pkgerrors "github.com/possibities/gin-core/pkg/errors"
	"github.com/possibities/gin-core/pkg/response"
)

type stubEnforcer struct {
	allowed bool
	err     error
	args    []any
}

func (s *stubEnforcer) Enforce(rvals ...any) (bool, error) {
	s.args = rvals
	return s.allowed, s.err
}

func TestRBACAllowsAuthorizedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	enforcer := &stubEnforcer{allowed: true}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(RoleKey, "admin")
		c.Next()
	})
	group := router.Group("/api/v1/admin")
	group.Use(RBAC(enforcer))
	group.GET("/session", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/session", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", recorder.Code)
	}
	if len(enforcer.args) != 3 || enforcer.args[0] != "admin" || enforcer.args[1] != "/api/v1/admin/session" || enforcer.args[2] != http.MethodGet {
		t.Fatalf("unexpected enforce args: %#v", enforcer.args)
	}
}

func TestRBACRejectsForbiddenRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	enforcer := &stubEnforcer{allowed: false}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(RoleKey, "member")
		c.Next()
	})
	group := router.Group("/api/v1/admin")
	group.Use(RBAC(enforcer))
	group.GET("/session", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/session", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}
}

func TestRBACReturnsUnifiedInternalErrorOnEnforcerFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	enforcer := &stubEnforcer{err: errors.New("casbin down")}
	router := gin.New()
	router.Use(RequestID(), func(c *gin.Context) {
		c.Set(RoleKey, "admin")
		c.Next()
	})
	group := router.Group("/api/v1/admin")
	group.Use(RBAC(enforcer))
	group.GET("/session", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/session", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}

	var body response.Body
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Code != pkgerrors.ErrInternal.Code || body.TraceID == "" {
		t.Fatalf("unexpected response body: %+v", body)
	}
}
