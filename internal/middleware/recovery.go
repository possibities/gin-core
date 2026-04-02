package middleware

import (
	"github.com/gin-gonic/gin"
	pkgerrors "github.com/possibities/gin-core/pkg/errors"
	pkglogger "github.com/possibities/gin-core/pkg/logger"
	"github.com/possibities/gin-core/pkg/response"
	"go.uber.org/zap"
)

func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		pkglogger.FromContext(c.Request.Context()).Error("panic recovered",
			zap.Any("panic", recovered),
			zap.String("path", c.Request.URL.Path),
		)
		response.Fail(c, pkgerrors.ErrInternal)
		c.Abort()
	})
}
