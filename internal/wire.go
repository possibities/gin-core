//go:build wireinject

package internal

import (
	"github.com/google/wire"
	"github.com/possibities/gin-core/internal/app"
	"github.com/possibities/gin-core/internal/dependency"
	"github.com/possibities/gin-core/internal/event"
	"github.com/possibities/gin-core/internal/handler"
	"github.com/possibities/gin-core/internal/repository"
	"github.com/possibities/gin-core/internal/router"
	"github.com/possibities/gin-core/internal/scheduler"
	"github.com/possibities/gin-core/internal/service"
	"github.com/possibities/gin-core/internal/worker"
	"github.com/possibities/gin-core/pkg/cache"
	"github.com/possibities/gin-core/pkg/config"
	pkgi18n "github.com/possibities/gin-core/pkg/i18n"
	pkgjwt "github.com/possibities/gin-core/pkg/jwt"
	pkglogger "github.com/possibities/gin-core/pkg/logger"
	"github.com/possibities/gin-core/pkg/metrics"
	"github.com/possibities/gin-core/pkg/mq"
	pkgtracing "github.com/possibities/gin-core/pkg/tracing"
)

var providerSet = wire.NewSet(
	pkglogger.New,
	worker.NewPoolProvider,
	wire.Bind(new(metrics.WorkerQueue), new(*worker.Pool)),
	metrics.New,
	pkgtracing.New,
	event.NewBus,
	event.NewOutboxDispatcher,
	wire.Bind(new(event.Publisher), new(*event.Bus)),
	repository.NewDB,
	repository.NewAuditLogRepository,
	repository.NewOutboxRepository,
	repository.NewTxManager,
	repository.NewUserRepository,
	cache.NewRedisClient,
	cache.NewDistributedLocker,
	cache.NewKeyspace,
	cache.NewReadThroughStore,
	cache.NewSlidingWindowLimiter,
	wire.Bind(new(cache.ReadStore), new(*cache.ReadThroughStore)),
	pkgi18n.New,
	mq.NewPublisher,
	pkgjwt.NewRedisTokenStore,
	wire.Bind(new(pkgjwt.BlacklistStore), new(*pkgjwt.RedisTokenStore)),
	wire.Bind(new(pkgjwt.RefreshTokenStore), new(*pkgjwt.RedisTokenStore)),
	pkgjwt.NewManager,
	dependency.NewRBACEnforcer,
	dependency.NewDependencyCheckers,
	handler.NewHealthHandler,
	handler.NewSessionHandler,
	service.NewAuditService,
	service.NewUserProfileUpdatedSubscriber,
	service.NewUserService,
	handler.NewUserHandler,
	handler.NewAdminHandler,
	scheduler.NewScheduler,
	router.NewEngine,
	app.NewHTTPServer,
	app.NewApp,
)

func InitializeApp(cfg *config.Config) (*app.App, func(), error) {
	panic(wire.Build(providerSet))
}
