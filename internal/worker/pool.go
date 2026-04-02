package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/possibities/gin-boilerplate/pkg/config"
	pkgerrors "github.com/possibities/gin-boilerplate/pkg/errors"
	pkglogger "github.com/possibities/gin-boilerplate/pkg/logger"
	"go.uber.org/zap"
)

type Task func(context.Context) error

type job struct {
	ctx  context.Context
	task Task
}

type Pool struct {
	logger        *zap.Logger
	jobs          chan job
	submitTimeout time.Duration

	mu     sync.RWMutex
	closed bool
	wg     sync.WaitGroup
}

func NewPool(cfg *config.Config, logger *zap.Logger) *Pool {
	pool := &Pool{
		logger:        logger,
		jobs:          make(chan job, cfg.Worker.QueueSize),
		submitTimeout: time.Duration(cfg.Worker.SubmitTimeoutMs) * time.Millisecond,
	}
	for i := 0; i < cfg.Worker.Workers; i++ {
		pool.wg.Add(1)
		go pool.runWorker()
	}
	return pool
}

func NewPoolProvider(cfg *config.Config, logger *zap.Logger) (*Pool, func(), error) {
	pool := NewPool(cfg, logger)
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = pool.Shutdown(ctx)
	}
	return pool, cleanup, nil
}

func (p *Pool) QueueDepth() int {
	return len(p.jobs)
}

func (p *Pool) Submit(ctx context.Context, task Task) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return pkgerrors.ErrServiceShuttingDown
	}

	submitCtx, cancel := context.WithTimeout(ctx, p.submitTimeout)
	defer cancel()

	select {
	case p.jobs <- job{ctx: ctx, task: task}:
		return nil
	case <-submitCtx.Done():
		if errors.Is(submitCtx.Err(), context.DeadlineExceeded) {
			return pkgerrors.ErrTooManyRequests
		}
		return submitCtx.Err()
	}
}

func (p *Pool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.jobs)
	p.mu.Unlock()

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Pool) runWorker() {
	defer p.wg.Done()

	for item := range p.jobs {
		if err := item.task(item.ctx); err != nil {
			pkglogger.WithTraceID(p.logger, pkglogger.TraceIDFromContext(item.ctx)).Error("worker task failed", zap.Error(err))
		}
	}
}
