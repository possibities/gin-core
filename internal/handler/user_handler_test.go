package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-boilerplate/internal/middleware"
	"github.com/possibities/gin-boilerplate/internal/service"
	pkgerrors "github.com/possibities/gin-boilerplate/pkg/errors"
	pkgi18n "github.com/possibities/gin-boilerplate/pkg/i18n"
)

type stubUserService struct {
	getProfile    func(ctx context.Context, userID uint) (*service.UserProfile, error)
	updateProfile func(ctx context.Context, userID uint, input service.UpdateUserProfileInput) (*service.UserProfile, error)
}

func (s *stubUserService) GetProfile(ctx context.Context, userID uint) (*service.UserProfile, error) {
	return s.getProfile(ctx, userID)
}

func (s *stubUserService) UpdateProfile(ctx context.Context, userID uint, input service.UpdateUserProfileInput) (*service.UserProfile, error) {
	return s.updateProfile(ctx, userID, input)
}

func TestUserHandlerMeReturnsCurrentProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUserHandler(&stubUserService{
		getProfile: func(context.Context, uint) (*service.UserProfile, error) {
			return &service.UserProfile{
				ID:       42,
				Email:    "alice@example.com",
				Name:     "Alice",
				Role:     "member",
				TenantID: "tenant-a",
			}, nil
		},
	})

	router := gin.New()
	router.GET("/api/v1/users/me", func(c *gin.Context) {
		c.Set(middleware.UserIDKey, uint(42))
		handler.Me(c)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if int(body["code"].(float64)) != 0 {
		t.Fatalf("expected code 0, got %+v", body)
	}
}

func TestUserHandlerMeRejectsMissingUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUserHandler(&stubUserService{
		getProfile: func(context.Context, uint) (*service.UserProfile, error) {
			return nil, nil
		},
	})

	router := gin.New()
	router.GET("/api/v1/users/me", handler.Me)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", recorder.Code)
	}
}

func TestUserHandlerMePropagatesServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUserHandler(&stubUserService{
		getProfile: func(context.Context, uint) (*service.UserProfile, error) {
			return nil, pkgerrors.ErrNotFound
		},
	})

	router := gin.New()
	router.GET("/api/v1/users/me", func(c *gin.Context) {
		c.Set(middleware.UserIDKey, uint(42))
		handler.Me(c)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recorder.Code)
	}
}

func TestUserHandlerUpdateMeBindsAndReturnsProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var captured service.UpdateUserProfileInput
	handler := NewUserHandler(&stubUserService{
		getProfile: func(context.Context, uint) (*service.UserProfile, error) {
			return nil, nil
		},
		updateProfile: func(_ context.Context, _ uint, input service.UpdateUserProfileInput) (*service.UserProfile, error) {
			captured = input
			return &service.UserProfile{
				ID:       42,
				Email:    "alice@example.com",
				Name:     "Alice Updated",
				Role:     "member",
				TenantID: "tenant-b",
			}, nil
		},
	})

	router := gin.New()
	router.PATCH("/api/v1/users/me", func(c *gin.Context) {
		c.Set(middleware.UserIDKey, uint(42))
		handler.UpdateMe(c)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"email":"alice@example.com","name":"Alice Updated","tenant_id":"tenant-b"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if captured.Email != "alice@example.com" || captured.Name != "Alice Updated" || captured.TenantID != "tenant-b" {
		t.Fatalf("unexpected update input: %+v", captured)
	}
}

func TestUserHandlerUpdateMeRejectsInvalidBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUserHandler(&stubUserService{
		getProfile: func(context.Context, uint) (*service.UserProfile, error) {
			return nil, nil
		},
		updateProfile: func(context.Context, uint, service.UpdateUserProfileInput) (*service.UserProfile, error) {
			return nil, nil
		},
	})

	router := gin.New()
	router.PATCH("/api/v1/users/me", func(c *gin.Context) {
		c.Set(middleware.UserIDKey, uint(42))
		handler.UpdateMe(c)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"email":"bad","name":""}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestUserHandlerUpdateMeLocalizesValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	translator, err := pkgi18n.New()
	if err != nil {
		t.Fatalf("new translator: %v", err)
	}

	handler := NewUserHandler(&stubUserService{
		getProfile: func(context.Context, uint) (*service.UserProfile, error) {
			return nil, nil
		},
		updateProfile: func(context.Context, uint, service.UpdateUserProfileInput) (*service.UserProfile, error) {
			return nil, nil
		},
	})

	router := gin.New()
	router.Use(middleware.Locale(translator))
	router.PATCH("/api/v1/users/me", func(c *gin.Context) {
		c.Set(middleware.UserIDKey, uint(42))
		handler.UpdateMe(c)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"email":"alice@example.com","name":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "zh-CN")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["message"] != "name 为必填项" {
		t.Fatalf("expected localized validation message, got %+v", body)
	}
}
