package handler

import (
	"sync/atomic"

	"github.com/gin-gonic/gin"
	pkgerrors "github.com/possibities/gin-core/pkg/errors"
	pkglogger "github.com/possibities/gin-core/pkg/logger"
	"github.com/possibities/gin-core/pkg/response"
	"go.uber.org/zap"
)

type DependencyChecker interface {
	Name() string
	Check() error
}

type HealthHandler struct {
	shuttingDown atomic.Bool
	checkers     []DependencyChecker
}

func NewHealthHandler(checkers []DependencyChecker) *HealthHandler {
	return &HealthHandler{checkers: checkers}
}

func (h *HealthHandler) SetShuttingDown(shuttingDown bool) {
	h.shuttingDown.Store(shuttingDown)
}

// Healthz godoc
// @Summary Liveness probe
// @Tags health
// @Produce json
// @Success 200 {object} successResponseHealth
// @Failure 503 {object} errorResponse
// @Router /healthz [get]
func (h *HealthHandler) Healthz(c *gin.Context) {
	if h.shuttingDown.Load() {
		response.Fail(c, pkgerrors.ErrServiceShuttingDown)
		return
	}
	response.Success(c, gin.H{"status": "ok"})
}

// Readyz godoc
// @Summary Readiness probe
// @Tags health
// @Produce json
// @Success 200 {object} successResponseHealth
// @Failure 503 {object} errorResponse
// @Router /readyz [get]
func (h *HealthHandler) Readyz(c *gin.Context) {
	if h.shuttingDown.Load() {
		response.Fail(c, pkgerrors.ErrServiceShuttingDown)
		return
	}

	for _, checker := range h.checkers {
		if err := checker.Check(); err != nil {
			pkglogger.FromContext(c.Request.Context()).Warn("dependency not ready",
				zap.String("dependency", checker.Name()),
				zap.Error(err),
			)
			response.Fail(c, pkgerrors.ErrDependencyNotReady)
			return
		}
	}

	response.Success(c, gin.H{"status": "ready"})
}
