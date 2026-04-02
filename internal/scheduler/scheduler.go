package scheduler

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/possibities/gin-boilerplate/internal/event"
	"github.com/possibities/gin-boilerplate/pkg/cache"
	"github.com/possibities/gin-boilerplate/pkg/config"
	pkglogger "github.com/possibities/gin-boilerplate/pkg/logger"
	"github.com/possibities/gin-boilerplate/pkg/metrics"
	"go.uber.org/zap"
)

const outboxDispatchTask = "system:outbox_dispatch"
const outboxDispatchLockResource = "scheduler:outbox-dispatch"

type outboxDispatcher interface {
	DispatchOnce(ctx context.Context) error
}

type distributedLocker interface {
	WithLock(ctx context.Context, resource string, ttl time.Duration, fn func(context.Context) error) (bool, error)
}

type Scheduler struct {
	cfg      *config.Config
	logger   *zap.Logger
	locker   distributedLocker
	outbox   outboxDispatcher
	metrics  *metrics.Registry
	server   *asynq.Server
	schedule *asynq.Scheduler
	mux      *asynq.ServeMux
	noop     bool
	started  bool
}

func NewScheduler(cfg *config.Config, logger *zap.Logger, locker *cache.DistributedLocker, outbox *event.OutboxDispatcher, metricsRegistry *metrics.Registry) (*Scheduler, error) {
	if !cfg.Scheduler.Enabled {
		return &Scheduler{noop: true}, nil
	}

	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}

	server := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.Scheduler.Concurrency,
		Queues: map[string]int{
			cfg.Scheduler.Queue: 1,
		},
		Logger: asynqLogger{logger: logger.Named("asynq")},
	})
	periodic := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{
		Location: time.UTC,
		Logger:   asynqLogger{logger: logger.Named("asynq")},
	})

	mux := asynq.NewServeMux()
	s := &Scheduler{
		cfg:      cfg,
		logger:   logger,
		locker:   locker,
		outbox:   outbox,
		metrics:  metricsRegistry,
		server:   server,
		schedule: periodic,
		mux:      mux,
	}

	mux.HandleFunc(outboxDispatchTask, s.handleOutboxDispatch)
	if cfg.MQ.Enabled {
		if _, err := periodic.Register(
			cfg.Scheduler.OutboxDispatchCron,
			asynq.NewTask(outboxDispatchTask, nil),
			asynq.Queue(cfg.Scheduler.Queue),
			asynq.Timeout(time.Duration(cfg.Scheduler.TaskTimeoutSec)*time.Second),
			asynq.MaxRetry(cfg.MQ.MaxAttempts),
		); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *Scheduler) Start() error {
	if s == nil || s.noop || s.started {
		return nil
	}
	if err := s.server.Start(s.mux); err != nil {
		return err
	}
	if err := s.schedule.Start(); err != nil {
		s.server.Shutdown()
		return err
	}
	s.started = true
	return nil
}

func (s *Scheduler) Shutdown(ctx context.Context) error {
	if s == nil || s.noop || !s.started {
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.schedule.Shutdown()
		s.server.Shutdown()
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) handleOutboxDispatch(ctx context.Context, _ *asynq.Task) error {
	startedAt := time.Now()
	result := "success"
	defer func() {
		if s.metrics != nil {
			s.metrics.ObserveSchedulerTask(outboxDispatchTask, result, time.Since(startedAt))
		}
	}()

	if s.outbox == nil {
		result = "noop"
		return nil
	}

	ttl := time.Duration(s.cfg.Scheduler.LockTTLSec) * time.Second
	acquired, err := s.locker.WithLock(ctx, outboxDispatchLockResource, ttl, func(lockCtx context.Context) error {
		return s.outbox.DispatchOnce(pkglogger.WithRequestContext(lockCtx, pkglogger.FromContext(lockCtx), pkglogger.TraceIDFromContext(lockCtx)))
	})
	if err != nil {
		result = "error"
		return err
	}
	if !acquired {
		result = "skipped"
		s.logger.Debug("skip scheduled task because lock is held", zap.String("task", outboxDispatchTask))
	}
	return nil
}

type asynqLogger struct {
	logger *zap.Logger
}

func (l asynqLogger) Debug(args ...any) {
	l.logger.Debug("asynq", zap.Any("args", args))
}

func (l asynqLogger) Info(args ...any) {
	l.logger.Info("asynq", zap.Any("args", args))
}

func (l asynqLogger) Warn(args ...any) {
	l.logger.Warn("asynq", zap.Any("args", args))
}

func (l asynqLogger) Error(args ...any) {
	l.logger.Error("asynq", zap.Any("args", args))
}

func (l asynqLogger) Fatal(args ...any) {
	l.logger.Fatal("asynq", zap.Any("args", args))
}
