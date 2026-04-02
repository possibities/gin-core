package app

import (
	"context"
	"errors"
	"net/http"

	"github.com/possibities/gin-core/internal/event"
	"github.com/possibities/gin-core/internal/handler"
	"github.com/possibities/gin-core/internal/scheduler"
	"github.com/possibities/gin-core/internal/service"
	"github.com/possibities/gin-core/internal/worker"
	"go.uber.org/zap"
)

type App struct {
	Server        *http.Server
	HealthHandler *handler.HealthHandler
	Scheduler     *scheduler.Scheduler
	Outbox        *event.OutboxDispatcher
	Workers       *worker.Pool
	Logger        *zap.Logger
}

func NewApp(
	server *http.Server,
	healthHandler *handler.HealthHandler,
	scheduler *scheduler.Scheduler,
	outbox *event.OutboxDispatcher,
	workers *worker.Pool,
	_ *service.UserProfileUpdatedSubscriber,
	logger *zap.Logger,
) *App {
	return &App{
		Server:        server,
		HealthHandler: healthHandler,
		Scheduler:     scheduler,
		Outbox:        outbox,
		Workers:       workers,
		Logger:        logger,
	}
}

func (a *App) Start() error {
	if a.Outbox != nil {
		a.Outbox.Start()
	}
	if a.Scheduler != nil {
		return a.Scheduler.Start()
	}
	return nil
}

func (a *App) Shutdown(ctx context.Context) error {
	var shutdownErr error
	if a.Scheduler != nil {
		shutdownErr = errors.Join(shutdownErr, a.Scheduler.Shutdown(ctx))
	}
	if a.Outbox != nil {
		shutdownErr = errors.Join(shutdownErr, a.Outbox.Shutdown(ctx))
	}
	if a.Workers != nil {
		shutdownErr = errors.Join(shutdownErr, a.Workers.Shutdown(ctx))
	}
	return shutdownErr
}
