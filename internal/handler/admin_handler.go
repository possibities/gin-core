package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-boilerplate/internal/middleware"
	"github.com/possibities/gin-boilerplate/internal/service"
	pkglogger "github.com/possibities/gin-boilerplate/pkg/logger"
	"github.com/possibities/gin-boilerplate/pkg/response"
	"go.uber.org/zap"
)

type AdminHandler struct {
	audit *service.AuditService
}

func NewAdminHandler(audit *service.AuditService) *AdminHandler {
	return &AdminHandler{audit: audit}
}

// Current godoc
// @Summary Current admin session
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} successResponseSession
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/admin/session [get]
func (h *AdminHandler) Current(c *gin.Context) {
	userID, _ := c.Get(middleware.UserIDKey)
	role, _ := c.Get(middleware.RoleKey)
	tenantID, _ := c.Get(middleware.TenantIDKey)
	traceID, _ := c.Get(middleware.TraceIDKey)

	actorUserID, _ := userID.(uint)
	if err := h.audit.Record(c.Request.Context(), service.AuditRecord{
		ActorUserID:  actorUserID,
		Action:       "admin.session.view",
		ResourceType: "session",
		ResourceID:   "current",
		BeforeState:  "",
		AfterState:   `{"scope":"admin"}`,
		IPAddress:    c.ClientIP(),
		TraceID:      toString(traceID),
	}); err != nil {
		pkglogger.FromContext(c.Request.Context()).Error("write audit log failed", zap.Error(err))
		response.Fail(c, err)
		return
	}

	response.Success(c, gin.H{
		"user_id":   userID,
		"role":      role,
		"tenant_id": tenantID,
		"scope":     "admin",
	})
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
