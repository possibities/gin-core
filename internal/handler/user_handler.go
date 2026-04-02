package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-core/internal/middleware"
	"github.com/possibities/gin-core/internal/service"
	pkgerrors "github.com/possibities/gin-core/pkg/errors"
	"github.com/possibities/gin-core/pkg/response"
)

type UserHandler struct {
	users service.UserService
}

type updateUserProfileRequest struct {
	Email    string `json:"email" binding:"required,email,max=255"`
	Name     string `json:"name" binding:"required,max=128"`
	TenantID string `json:"tenant_id" binding:"max=64"`
}

func NewUserHandler(users service.UserService) *UserHandler {
	return &UserHandler{users: users}
}

// Me godoc
// @Summary Current user profile
// @Tags users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} successResponseUserProfile
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/users/me [get]
func (h *UserHandler) Me(c *gin.Context) {
	userID, ok := c.Get(middleware.UserIDKey)
	if !ok {
		response.Fail(c, pkgerrors.ErrUnauthorized)
		return
	}

	subjectID, ok := userID.(uint)
	if !ok || subjectID == 0 {
		response.Fail(c, pkgerrors.ErrUnauthorized)
		return
	}

	profile, err := h.users.GetProfile(c.Request.Context(), subjectID)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.Success(c, profile)
}

// UpdateMe godoc
// @Summary Update current user profile
// @Tags users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body updateUserProfileRequest true "Update user profile request"
// @Success 200 {object} successResponseUserProfile
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/users/me [patch]
func (h *UserHandler) UpdateMe(c *gin.Context) {
	userID, ok := c.Get(middleware.UserIDKey)
	if !ok {
		response.Fail(c, pkgerrors.ErrUnauthorized)
		return
	}

	subjectID, ok := userID.(uint)
	if !ok || subjectID == 0 {
		response.Fail(c, pkgerrors.ErrUnauthorized)
		return
	}

	var req updateUserProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, err)
		return
	}

	profile, err := h.users.UpdateProfile(c.Request.Context(), subjectID, service.UpdateUserProfileInput{
		ActorUserID: subjectID,
		Email:       req.Email,
		Name:        req.Name,
		TenantID:    req.TenantID,
		IPAddress:   c.ClientIP(),
		TraceID:     responseTraceID(c),
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.Success(c, profile)
}

func responseTraceID(c *gin.Context) string {
	if traceID, ok := c.Get(middleware.TraceIDKey); ok {
		if s, ok := traceID.(string); ok {
			return s
		}
	}
	return ""
}
