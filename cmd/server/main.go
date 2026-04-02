package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	docs "github.com/possibities/gin-core/docs"
	"github.com/possibities/gin-core/internal"
	internalmigration "github.com/possibities/gin-core/internal/migration"
	"github.com/possibities/gin-core/pkg/config"
	"go.uber.org/zap"
)

// @title Gin Boilerplate API
// @version 1.0
// @description Enterprise Gin boilerplate API.
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}

	docs.SwaggerInfo.Title = "Gin Boilerplate API"
	docs.SwaggerInfo.Description = "Enterprise Gin boilerplate API."
	docs.SwaggerInfo.Version = "1.0"
	docs.SwaggerInfo.BasePath = "/"
	if cfg.App.Env == "prod" {
		docs.SwaggerInfo.Host = ""
	} else {
		docs.SwaggerInfo.Host = fmt.Sprintf("%s:%d", cfg.App.Host, cfg.App.Port)
	}
	migrator := internalmigration.New(cfg)
	if migrator.AutoRunEnabled() {
		migrateCtx, migrateCancel := context.WithTimeout(context.Background(), migrator.Timeout())
		defer migrateCancel()
		if err := migrator.Up(migrateCtx); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			panic(fmt.Errorf("run migrations: %w", err))
		}
	}

	app, cleanup, err := internal.InitializeApp(cfg)
	if err != nil {
		panic(fmt.Errorf("initialize app: %w", err))
	}
	defer cleanup()
	if err := app.Start(); err != nil {
		panic(fmt.Errorf("start app: %w", err))
	}

	errCh := make(chan error, 1)
	go func() {
		if serveErr := app.Server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case runErr := <-errCh:
		panic(fmt.Errorf("server failed: %w", runErr))
	case <-sigCh:
	}

	app.HealthHandler.SetShuttingDown(true)

	// Total shutdown budget must be less than K8s terminationGracePeriodSeconds (default 30s).
	totalBudget := time.Duration(cfg.App.ShutdownTimeoutSec) * time.Second
	deadline := time.Now().Add(totalBudget)

	logger := app.Logger

	// Step 1: Stop accepting new connections and drain in-flight requests (15s budget).
	httpTimeout := 15 * time.Second
	if remaining := time.Until(deadline); httpTimeout > remaining {
		httpTimeout = remaining
	}
	httpCtx, httpCancel := context.WithTimeout(context.Background(), httpTimeout)
	defer httpCancel()
	if err = app.Server.Shutdown(httpCtx); err != nil {
		logger.Warn("http server shutdown incomplete", zap.Error(err))
	}

	// Step 2: Drain worker pool, scheduler, outbox (10s budget).
	workerTimeout := 10 * time.Second
	if remaining := time.Until(deadline); workerTimeout > remaining {
		workerTimeout = remaining
	}
	workerCtx, workerCancel := context.WithTimeout(context.Background(), workerTimeout)
	defer workerCancel()
	if err = app.Shutdown(workerCtx); err != nil {
		logger.Warn("worker shutdown incomplete", zap.Error(err))
	}

	// Step 3: Close DB, Redis, flush logger (shared 5s budget via cleanup).
	// cleanup() is deferred from InitializeApp and handles resource teardown.

	if time.Now().After(deadline) {
		logger.Error("shutdown budget exhausted, force exiting")
		os.Exit(1)
	}
}
