package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	pkgerrors "github.com/possibities/gin-boilerplate/pkg/errors"
	pkgjwt "github.com/possibities/gin-boilerplate/pkg/jwt"
	"github.com/possibities/gin-boilerplate/pkg/response"
)

const (
	UserIDKey   = "user_id"
	RoleKey     = "role"
	TenantIDKey = "tenant_id"
)

func Auth(tokenManager *pkgjwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		rawToken, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok || strings.TrimSpace(rawToken) == "" {
			response.Fail(c, pkgerrors.ErrUnauthorized)
			c.Abort()
			return
		}

		claims, err := tokenManager.AuthenticateAccessToken(c.Request.Context(), strings.TrimSpace(rawToken))
		if err != nil {
			response.Fail(c, err)
			c.Abort()
			return
		}

		c.Set(UserIDKey, claims.UserID)
		c.Set(RoleKey, claims.Role)
		c.Set(TenantIDKey, claims.TenantID)
		c.Next()
	}
}
