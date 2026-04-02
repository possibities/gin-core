package event

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/possibities/gin-boilerplate/internal/model"
	"github.com/possibities/gin-boilerplate/internal/repository"
	"github.com/possibities/gin-boilerplate/pkg/config"
	"github.com/possibities/gin-boilerplate/pkg/metrics"
	"github.com/possibities/gin-boilerplate/pkg/mq"
	"go.uber.org/zap"
)

type OutboxDispatcher struct {
	cfg       *config.Config
	logger    *zap.Logger
	outbox    repository.OutboxRepository
	publisher mq.Publisher
	metrics   *metrics.Registry
	owner     string
	now       func() time.Time

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func NewOutboxDispatcher(cfg *config.Config, logger *zap.Logger, outbox repository.OutboxRepository, publisher mq.Publisher, metricsRegistry *metrics.Registry) *OutboxDispatcher {
	return &OutboxDispatcher{
		cfg:       cfg,
		logger:    logger,
		outbox:    outbox,
		publisher: publisher,
		metrics:   metricsRegistry,
		owner:     uuid.NewString(),
		now:       time.Now,
	}
}

func (d *OutboxDispatcher) Start() {
	if !d.cfg.MQ.Enabled || d.cfg.Scheduler.Enabled {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	d.cancel = cancel
	d.done = done

	go func() {
		defer close(done)
		d.run(ctx)
	}()
}

func (d *OutboxDispatcher) Shutdown(ctx context.Context) error {
	d.mu.Lock()
	cancel := d.cancel
	done := d.done
	d.cancel = nil
	d.done = nil
	d.mu.Unlock()

	if cancel == nil || done == nil {
		return nil
	}
	cancel()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *OutboxDispatcher) run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(d.cfg.MQ.PollIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		if err := d.DispatchOnce(ctx); err != nil && ctx.Err() == nil {
			d.logger.Warn("dispatch outbox events failed", zap.Error(err))
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *OutboxDispatcher) DispatchOnce(ctx context.Context) error {
	startedAt := time.Now()
	now := d.now().UTC()
	staleBefore := now.Add(-time.Duration(d.cfg.MQ.LockTimeoutSec) * time.Second)

	acquireCtx, cancelAcquire := context.WithTimeout(ctx, time.Duration(d.cfg.MQ.OperationTimeoutMs)*time.Millisecond)
	events, err := d.outbox.AcquirePending(acquireCtx, d.cfg.MQ.BatchSize, now, staleBefore, d.owner)
	cancelAcquire()
	if err != nil {
		d.observeDispatch("acquire_failed", startedAt)
		return err
	}
	if len(events) == 0 {
		d.observeDispatch("empty", startedAt)
		return nil
	}

	var firstErr error
	for _, evt := range events {
		if err := d.dispatchEvent(ctx, evt); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		d.observeDispatch("failed", startedAt)
		return firstErr
	}

	d.observeDispatch("success", startedAt)
	return nil
}

func (d *OutboxDispatcher) dispatchEvent(ctx context.Context, evt *model.OutboxEvent) error {
	publishCtx, cancelPublish := context.WithTimeout(ctx, time.Duration(d.cfg.MQ.PublishTimeoutMs)*time.Millisecond)
	err := d.publisher.Publish(publishCtx, mq.Message{
		ID:        evt.ID,
		Topic:     evt.Topic,
		Payload:   evt.Payload,
		Timestamp: evt.CreatedAt.UTC(),
	})
	cancelPublish()
	if err != nil {
		d.metrics.ObserveOutboxPublishAttempt(evt.Topic, "failed")
		attempts := evt.Attempts + 1
		dead := attempts >= d.cfg.MQ.MaxAttempts
		backoff := retryBackoff(attempts)
		updateCtx, cancelUpdate := context.WithTimeout(ctx, time.Duration(d.cfg.MQ.OperationTimeoutMs)*time.Millisecond)
		updateErr := d.outbox.MarkFailed(updateCtx, evt.ID, attempts, err.Error(), d.now().Add(backoff), dead)
		cancelUpdate()
		if updateErr != nil {
			return fmt.Errorf("publish outbox event: %w; mark failed: %v", err, updateErr)
		}
		d.logger.Warn("publish outbox event failed",
			zap.String("event_id", evt.ID),
			zap.String("topic", evt.Topic),
			zap.Int("attempts", attempts),
			zap.Bool("dead", dead),
			zap.Error(err),
		)
		return err
	}

	d.metrics.ObserveOutboxPublishAttempt(evt.Topic, "published")
	updateCtx, cancelUpdate := context.WithTimeout(ctx, time.Duration(d.cfg.MQ.OperationTimeoutMs)*time.Millisecond)
	updateErr := d.outbox.MarkPublished(updateCtx, evt.ID, d.now())
	cancelUpdate()
	if updateErr != nil {
		return updateErr
	}
	return nil
}

func (d *OutboxDispatcher) observeDispatch(result string, startedAt time.Time) {
	if d.metrics == nil {
		return
	}
	d.metrics.ObserveOutboxDispatch(result, time.Since(startedAt))
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	backoff := time.Second * time.Duration(1<<(attempt-1))
	if backoff > 5*time.Minute {
		return 5 * time.Minute
	}
	return backoff
}
