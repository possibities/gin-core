package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-boilerplate/pkg/metrics"
)

func Metrics(registry *metrics.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		registry.ObserveHTTPRequest(c.Request.Method, path, c.Writer.Status(), time.Since(start))
	}
}
