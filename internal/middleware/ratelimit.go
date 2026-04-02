package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-boilerplate/pkg/cache"
	"github.com/possibities/gin-boilerplate/pkg/config"
	pkgerrors "github.com/possibities/gin-boilerplate/pkg/errors"
	pkglogger "github.com/possibities/gin-boilerplate/pkg/logger"
	"github.com/possibities/gin-boilerplate/pkg/response"
	"go.uber.org/zap"
)

func RateLimiter(cfg *config.Config, keys *cache.Keyspace, limiter cache.WindowLimiter) gin.HandlerFunc {
	if !cfg.RateLimit.Enabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return func(c *gin.Context) {
		key := keys.RateLimitIP(c.ClientIP())
		limit := cfg.RateLimit.RPS * cfg.RateLimit.WindowSec
		if limit <= 0 {
			limit = cfg.RateLimit.Burst
		}
		result, err := limiter.Check(c.Request.Context(), key, limit, time.Duration(cfg.RateLimit.WindowSec)*time.Second)
		if err != nil {
			pkglogger.FromContext(c.Request.Context()).Warn("redis rate limiter degraded open",
				zap.Error(err),
				zap.String("client_ip", c.ClientIP()),
			)
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))

		if !result.Allowed {
			retryAfter := time.Until(result.ResetAt).Seconds()
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.Itoa(int(retryAfter)))
			response.Fail(c, pkgerrors.ErrTooManyRequests)
			c.Abort()
			return
		}
		c.Next()
	}
}
