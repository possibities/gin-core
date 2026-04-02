package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	pkglogger "github.com/possibities/gin-core/pkg/logger"
	pkgtracing "github.com/possibities/gin-core/pkg/tracing"
	"go.uber.org/zap"
)

const TraceIDKey = "trace_id"
const LoggerKey = "logger"

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID, ok := pkgtracing.NormalizeTraceID(strings.TrimSpace(c.GetHeader("X-Trace-Id")))
		if !ok {
			traceID = pkgtracing.NewTraceID()
		}

		requestLogger := pkglogger.WithTraceID(zap.L(), traceID)
		c.Set(TraceIDKey, traceID)
		c.Set(LoggerKey, requestLogger)
		c.Request = c.Request.WithContext(pkglogger.WithRequestContext(c.Request.Context(), requestLogger, traceID))
		c.Writer.Header().Set("X-Trace-Id", traceID)
		c.Next()
	}
}
