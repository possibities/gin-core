package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	pkglogger "github.com/possibities/gin-core/pkg/logger"
	"go.uber.org/zap"
)

func ZapLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		z := pkglogger.FromContext(c.Request.Context())
		if len(c.Errors) > 0 {
			z.Error("request completed with errors",
				zap.Int("status", status),
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.String("client_ip", c.ClientIP()),
				zap.Duration("latency", latency),
				zap.String("error", c.Errors.String()),
			)
			return
		}

		z.Info("request completed",
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("client_ip", c.ClientIP()),
			zap.Duration("latency", latency),
		)
	}
}
