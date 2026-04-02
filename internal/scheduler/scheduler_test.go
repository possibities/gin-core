package scheduler

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/possibities/gin-core/pkg/config"
	"github.com/possibities/gin-core/pkg/metrics"
	"go.uber.org/zap"
)

type stubLocker struct {
	withLock func(ctx context.Context, resource string, ttl time.Duration, fn func(context.Context) error) (bool, error)
}

type stubOutbox struct {
	dispatchOnce func(ctx context.Context) error
}

func (l stubLocker) WithLock(ctx context.Context, resource string, ttl time.Duration, fn func(context.Context) error) (bool, error) {
	return l.withLock(ctx, resource, ttl, fn)
}

func (o stubOutbox) DispatchOnce(ctx context.Context) error {
	return o.dispatchOnce(ctx)
}

func TestHandleOutboxDispatchRunsUnderLock(t *testing.T) {
	registry := metrics.New(nil)
	var dispatched bool
	s := &Scheduler{
		cfg: &config.Config{
			Scheduler: config.SchedulerConfig{
				LockTTLSec: 30,
			},
		},
		logger:  zap.NewNop(),
		metrics: registry,
		locker: stubLocker{
			withLock: func(ctx context.Context, resource string, ttl time.Duration, fn func(context.Context) error) (bool, error) {
				if resource != outboxDispatchLockResource {
					t.Fatalf("unexpected lock resource %q", resource)
				}
				if ttl != 30*time.Second {
					t.Fatalf("unexpected ttl %v", ttl)
				}
				return true, fn(ctx)
			},
		},
		outbox: stubOutbox{
			dispatchOnce: func(context.Context) error {
				dispatched = true
				return nil
			},
		},
	}

	if err := s.handleOutboxDispatch(context.Background(), asynq.NewTask(outboxDispatchTask, nil)); err != nil {
		t.Fatalf("handleOutboxDispatch() error = %v", err)
	}
	if !dispatched {
		t.Fatal("expected outbox dispatch to run")
	}

	body := schedulerMetricsBody(t, registry)
	checks := []string{
		"scheduler_task_runs_total{result=\"success\",task=\"system:outbox_dispatch\"} 1",
		"scheduler_task_duration_seconds_bucket{result=\"success\",task=\"system:outbox_dispatch\",le=\"0.025\"} 1",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("expected metrics output to contain %q", want)
		}
	}
}

func TestHandleOutboxDispatchSkipsWhenLockUnavailable(t *testing.T) {
	registry := metrics.New(nil)
	s := &Scheduler{
		cfg:     &config.Config{Scheduler: config.SchedulerConfig{LockTTLSec: 30}},
		logger:  zap.NewNop(),
		metrics: registry,
		locker: stubLocker{
			withLock: func(context.Context, string, time.Duration, func(context.Context) error) (bool, error) {
				return false, nil
			},
		},
		outbox: stubOutbox{
			dispatchOnce: func(context.Context) error {
				t.Fatal("dispatch should not run without lock")
				return nil
			},
		},
	}

	if err := s.handleOutboxDispatch(context.Background(), asynq.NewTask(outboxDispatchTask, nil)); err != nil {
		t.Fatalf("handleOutboxDispatch() error = %v", err)
	}

	body := schedulerMetricsBody(t, registry)
	if !strings.Contains(body, "scheduler_task_runs_total{result=\"skipped\",task=\"system:outbox_dispatch\"} 1") {
		t.Fatal("expected skipped scheduler metric to be recorded")
	}
}

func TestHandleOutboxDispatchReturnsDispatchError(t *testing.T) {
	registry := metrics.New(nil)
	s := &Scheduler{
		cfg:     &config.Config{Scheduler: config.SchedulerConfig{LockTTLSec: 30}},
		logger:  zap.NewNop(),
		metrics: registry,
		locker: stubLocker{
			withLock: func(ctx context.Context, _ string, _ time.Duration, fn func(context.Context) error) (bool, error) {
				return true, fn(ctx)
			},
		},
		outbox: stubOutbox{
			dispatchOnce: func(context.Context) error {
				return errors.New("dispatch failed")
			},
		},
	}

	if err := s.handleOutboxDispatch(context.Background(), asynq.NewTask(outboxDispatchTask, nil)); err == nil {
		t.Fatal("expected dispatch error")
	}

	body := schedulerMetricsBody(t, registry)
	if !strings.Contains(body, "scheduler_task_runs_total{result=\"error\",task=\"system:outbox_dispatch\"} 1") {
		t.Fatal("expected error scheduler metric to be recorded")
	}
}

func schedulerMetricsBody(t *testing.T, registry *metrics.Registry) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	registry.Handler().ServeHTTP(recorder, req)
	return recorder.Body.String()
}
