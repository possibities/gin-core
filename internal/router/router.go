package router

import (
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-boilerplate/internal/handler"
	"github.com/possibities/gin-boilerplate/internal/middleware"
	"github.com/possibities/gin-boilerplate/pkg/cache"
	"github.com/possibities/gin-boilerplate/pkg/config"
	pkgi18n "github.com/possibities/gin-boilerplate/pkg/i18n"
	pkgjwt "github.com/possibities/gin-boilerplate/pkg/jwt"
	"github.com/possibities/gin-boilerplate/pkg/metrics"
	pkgtracing "github.com/possibities/gin-boilerplate/pkg/tracing"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func NewEngine(
	cfg *config.Config,
	healthHandler *handler.HealthHandler,
	sessionHandler *handler.SessionHandler,
	userHandler *handler.UserHandler,
	adminHandler *handler.AdminHandler,
	keys *cache.Keyspace,
	rateLimiter *cache.SlidingWindowLimiter,
	tokenManager *pkgjwt.Manager,
	rbacEnforcer *casbin.SyncedEnforcer,
	translator *pkgi18n.Translator,
	metricsRegistry *metrics.Registry,
	tracingProvider *pkgtracing.Provider,
) *gin.Engine {
	r := gin.New()
	r.Use(
		middleware.Recovery(),
		middleware.RequestID(),
		middleware.Timeout(cfg),
		middleware.Tracing(tracingProvider),
		middleware.Metrics(metricsRegistry),
		middleware.ZapLogger(),
		middleware.Locale(translator),
		middleware.CORS(),
		middleware.RateLimiter(cfg, keys, rateLimiter),
	)

	r.GET("/healthz", healthHandler.Healthz)
	r.GET("/readyz", healthHandler.Readyz)
	r.GET("/metrics", gin.WrapH(metricsRegistry.Handler()))
	if shouldExposeSwagger(cfg.App.Env) {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	}

	api := r.Group("/api/v1")
	api.Use(middleware.Auth(tokenManager))
	api.GET("/session", sessionHandler.Current)
	api.GET("/users/me", userHandler.Me)
	api.PATCH("/users/me", userHandler.UpdateMe)

	adminGroup := api.Group("/admin")
	if cfg.RBAC.Enabled {
		adminGroup.Use(middleware.RBAC(rbacEnforcer))
	}
	adminGroup.GET("/session", adminHandler.Current)
	return r
}

func shouldExposeSwagger(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "dev", "development", "staging":
		return true
	default:
		return false
	}
}
