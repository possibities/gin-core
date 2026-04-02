package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-boilerplate/pkg/config"
	pkgjwt "github.com/possibities/gin-boilerplate/pkg/jwt"
)

type testBlacklistStore struct{}

func (s *testBlacklistStore) BlacklistToken(_ context.Context, _ string, _ time.Duration) error {
	return nil
}
func (s *testBlacklistStore) IsBlacklisted(_ context.Context, _ string) (bool, error) {
	return false, nil
}

type testRefreshStore struct{}

func (s *testRefreshStore) StoreRefreshToken(_ context.Context, _ uint, _ string, _ time.Duration) error {
	return nil
}
func (s *testRefreshStore) HasRefreshToken(_ context.Context, _ uint, _ string) (bool, error) {
	return true, nil
}
func (s *testRefreshStore) DeleteRefreshToken(_ context.Context, _ uint, _ string) error { return nil }

func TestAuthMiddlewareRejectsMissingBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(Auth(newTestTokenManager()))
	router.GET("/api/v1/session", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", recorder.Code)
	}
}

func TestAuthMiddlewareInjectsClaimsIntoContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tokenManager := newTestTokenManager()
	pair, err := tokenManager.GenerateTokenPair(context.Background(), pkgjwt.Subject{
		UserID:   42,
		Role:     "admin",
		TenantID: "tenant-a",
	})
	if err != nil {
		t.Fatalf("GenerateTokenPair() error = %v", err)
	}

	router := gin.New()
	router.Use(Auth(tokenManager))
	router.GET("/api/v1/session", func(c *gin.Context) {
		if userID, ok := c.Get(UserIDKey); !ok || userID != uint(42) {
			t.Fatalf("expected user_id claim in context, got %v", userID)
		}
		if role, ok := c.Get(RoleKey); !ok || role != "admin" {
			t.Fatalf("expected role claim in context, got %v", role)
		}
		if tenantID, ok := c.Get(TenantIDKey); !ok || tenantID != "tenant-a" {
			t.Fatalf("expected tenant_id claim in context, got %v", tenantID)
		}
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
}

func newTestTokenManager() *pkgjwt.Manager {
	return pkgjwt.NewManager(&config.Config{
		App: config.AppConfig{Name: "gin-boilerplate"},
		JWT: config.JWTConfig{
			Issuer:        "gin-boilerplate",
			AccessTTLSec:  300,
			RefreshTTLSec: 600,
			AccessSecret:  "access-secret",
			RefreshSecret: "refresh-secret",
		},
	}, &testBlacklistStore{}, &testRefreshStore{})
}
