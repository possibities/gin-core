package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-core/internal/middleware"
	"github.com/possibities/gin-core/pkg/response"
)

type SessionHandler struct{}

func NewSessionHandler() *SessionHandler {
	return &SessionHandler{}
}

// Current godoc
// @Summary Current session
// @Tags session
// @Produce json
// @Security BearerAuth
// @Success 200 {object} successResponseSession
// @Failure 401 {object} errorResponse
// @Router /api/v1/session [get]
func (h *SessionHandler) Current(c *gin.Context) {
	userID, _ := c.Get(middleware.UserIDKey)
	role, _ := c.Get(middleware.RoleKey)
	tenantID, _ := c.Get(middleware.TenantIDKey)

	response.Success(c, gin.H{
		"user_id":   userID,
		"role":      role,
		"tenant_id": tenantID,
	})
}
