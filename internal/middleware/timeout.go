package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-boilerplate/pkg/config"
)

// Timeout sets a context deadline on every request so that individual call
// timeouts (DB, Redis, HTTP) are always bounded by a parent ceiling.
func Timeout(cfg *config.Config) gin.HandlerFunc {
	timeout := time.Duration(cfg.App.RequestTimeoutSec) * time.Second
	if timeout <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
