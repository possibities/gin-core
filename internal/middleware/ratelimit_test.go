package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-core/pkg/cache"
	"github.com/possibities/gin-core/pkg/config"
)

type stubWindowLimiter struct {
	allow bool
	err   error
	limit int
}

func (s *stubWindowLimiter) Allow(_ context.Context, _ string, limit int, _ time.Duration) (bool, error) {
	s.limit = limit
	return s.allow, s.err
}

func (s *stubWindowLimiter) Check(_ context.Context, _ string, limit int, window time.Duration) (*cache.RateLimitResult, error) {
	s.limit = limit
	if s.err != nil {
		return nil, s.err
	}
	remaining := 0
	if s.allow {
		remaining = limit - 1
	}
	return &cache.RateLimitResult{
		Allowed:   s.allow,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   time.Now().Add(window),
	}, nil
}

func TestRateLimiterRejectsWhenWindowIsExceeded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	limiter := &stubWindowLimiter{allow: false}
	cfg := &config.Config{
		App: config.AppConfig{Name: "gin-core"},
		RateLimit: config.RateLimitConfig{
			Enabled:   true,
			RPS:       3,
			Burst:     2,
			WindowSec: 1,
		},
	}
	router := gin.New()
	router.Use(RateLimiter(cfg, cache.NewKeyspace(cfg), limiter))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", recorder.Code)
	}
	if limiter.limit != 3 {
		t.Fatalf("expected limiter to use rps-derived limit, got %d", limiter.limit)
	}
}

func TestRateLimiterDegradesOpenOnBackendFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		App: config.AppConfig{Name: "gin-core"},
		RateLimit: config.RateLimitConfig{
			Enabled:   true,
			RPS:       2,
			Burst:     2,
			WindowSec: 1,
		},
	}
	router := gin.New()
	router.Use(RequestID(), RateLimiter(cfg, cache.NewKeyspace(cfg), &stubWindowLimiter{err: errors.New("redis down")}))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected fail-open behavior with status 204, got %d", recorder.Code)
	}
}
