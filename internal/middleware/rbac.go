package middleware

import (
	"github.com/gin-gonic/gin"
	pkgerrors "github.com/possibities/gin-boilerplate/pkg/errors"
	pkglogger "github.com/possibities/gin-boilerplate/pkg/logger"
	"github.com/possibities/gin-boilerplate/pkg/response"
	"go.uber.org/zap"
)

type RBACEnforcer interface {
	Enforce(rvals ...any) (bool, error)
}

type pathNormalizer interface {
	Normalize(path string) string
}

type defaultPathNormalizer struct{}

func (defaultPathNormalizer) Normalize(path string) string {
	if path == "" || path == "/" {
		return "/"
	}
	if len(path) > 1 && path[len(path)-1] == '/' {
		return path[:len(path)-1]
	}
	return path
}

func RBAC(enforcer RBACEnforcer) gin.HandlerFunc {
	return RBACWithNormalizer(enforcer, defaultPathNormalizer{})
}

func RBACWithNormalizer(enforcer RBACEnforcer, normalizer pathNormalizer) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleValue, ok := c.Get(RoleKey)
		role, roleOK := roleValue.(string)
		if !ok || !roleOK || role == "" {
			response.Fail(c, pkgerrors.ErrForbidden)
			c.Abort()
			return
		}

		allowed, err := enforcer.Enforce(role, normalizer.Normalize(c.FullPath()), c.Request.Method)
		if err != nil {
			pkglogger.FromContext(c.Request.Context()).Error("rbac enforcement failed",
				zap.Error(err),
				zap.String("role", role),
				zap.String("path", c.FullPath()),
				zap.String("method", c.Request.Method),
			)
			response.Fail(c, pkgerrors.ErrInternal)
			c.Abort()
			return
		}
		if !allowed {
			response.Fail(c, pkgerrors.ErrForbidden)
			c.Abort()
			return
		}

		c.Next()
	}
}
