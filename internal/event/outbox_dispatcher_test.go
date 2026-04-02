package event

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/possibities/gin-boilerplate/internal/model"
	"github.com/possibities/gin-boilerplate/internal/repository"
	"github.com/possibities/gin-boilerplate/pkg/config"
	"github.com/possibities/gin-boilerplate/pkg/metrics"
	"github.com/possibities/gin-boilerplate/pkg/mq"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type stubOutboxRepo struct {
	acquire       func(context.Context, int, time.Time, time.Time, string) ([]*model.OutboxEvent, error)
	markPublished func(context.Context, string, time.Time) error
	markFailed    func(context.Context, string, int, string, time.Time, bool) error
}

func (r *stubOutboxRepo) Create(context.Context, *model.OutboxEvent) error { return nil }
func (r *stubOutboxRepo) AcquirePending(ctx context.Context, limit int, now time.Time, staleBefore time.Time, owner string) ([]*model.OutboxEvent, error) {
	return r.acquire(ctx, limit, now, staleBefore, owner)
}
func (r *stubOutboxRepo) MarkPublished(ctx context.Context, id string, publishedAt time.Time) error {
	return r.markPublished(ctx, id, publishedAt)
}
func (r *stubOutboxRepo) MarkFailed(ctx context.Context, id string, attempts int, lastError string, nextAttemptAt time.Time, dead bool) error {
	return r.markFailed(ctx, id, attempts, lastError, nextAttemptAt, dead)
}
func (r *stubOutboxRepo) WithTx(*gorm.DB) repository.OutboxRepository { return r }

type stubMQPublisher struct {
	publish func(context.Context, mq.Message) error
}

func (p stubMQPublisher) Publish(ctx context.Context, message mq.Message) error {
	return p.publish(ctx, message)
}

func TestOutboxDispatcherMarksPublishedOnSuccess(t *testing.T) {
	registry := metrics.New(nil)
	repo := &stubOutboxRepo{
		acquire: func(context.Context, int, time.Time, time.Time, string) ([]*model.OutboxEvent, error) {
			return []*model.OutboxEvent{{
				ID:        "evt-1",
				Topic:     "user.profile.updated.v1",
				Payload:   []byte(`{"ok":true}`),
				CreatedAt: time.Unix(100, 0),
			}}, nil
		},
		markPublished: func(_ context.Context, id string, _ time.Time) error {
			if id != "evt-1" {
				t.Fatalf("unexpected published id %q", id)
			}
			return nil
		},
		markFailed: func(context.Context, string, int, string, time.Time, bool) error {
			t.Fatal("mark failed should not be called")
			return nil
		},
	}

	dispatcher := NewOutboxDispatcher(&config.Config{
		MQ: config.MQConfig{
			Enabled:            true,
			BatchSize:          10,
			MaxAttempts:        3,
			LockTimeoutSec:     30,
			OperationTimeoutMs: 100,
			PublishTimeoutMs:   100,
			PollIntervalMs:     1000,
		},
	}, zap.NewNop(), repo, stubMQPublisher{
		publish: func(_ context.Context, message mq.Message) error {
			if message.ID != "evt-1" {
				t.Fatalf("unexpected message id %q", message.ID)
			}
			return nil
		},
	}, registry)

	if err := dispatcher.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}

	body := metricsBody(t, registry)
	checks := []string{
		"outbox_publish_attempts_total{result=\"published\",topic=\"user.profile.updated.v1\"} 1",
		"outbox_dispatch_runs_total{result=\"success\"} 1",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("expected metrics output to contain %q", want)
		}
	}
}

func TestOutboxDispatcherMarksFailedOnPublishError(t *testing.T) {
	registry := metrics.New(nil)
	repo := &stubOutboxRepo{
		acquire: func(context.Context, int, time.Time, time.Time, string) ([]*model.OutboxEvent, error) {
			return []*model.OutboxEvent{{
				ID:       "evt-2",
				Topic:    "user.profile.updated.v1",
				Attempts: 1,
			}}, nil
		},
		markPublished: func(context.Context, string, time.Time) error {
			t.Fatal("mark published should not be called")
			return nil
		},
		markFailed: func(_ context.Context, id string, attempts int, lastError string, _ time.Time, dead bool) error {
			if id != "evt-2" || attempts != 2 || lastError == "" || dead {
				t.Fatalf("unexpected failure state: id=%q attempts=%d err=%q dead=%v", id, attempts, lastError, dead)
			}
			return nil
		},
	}

	dispatcher := NewOutboxDispatcher(&config.Config{
		MQ: config.MQConfig{
			Enabled:            true,
			BatchSize:          10,
			MaxAttempts:        3,
			LockTimeoutSec:     30,
			OperationTimeoutMs: 100,
			PublishTimeoutMs:   100,
			PollIntervalMs:     1000,
		},
	}, zap.NewNop(), repo, stubMQPublisher{
		publish: func(context.Context, mq.Message) error {
			return errors.New("broker down")
		},
	}, registry)

	if err := dispatcher.DispatchOnce(context.Background()); err == nil {
		t.Fatal("expected publish error")
	}

	body := metricsBody(t, registry)
	checks := []string{
		"outbox_publish_attempts_total{result=\"failed\",topic=\"user.profile.updated.v1\"} 1",
		"outbox_dispatch_runs_total{result=\"failed\"} 1",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("expected metrics output to contain %q", want)
		}
	}
}

func TestOutboxDispatcherRecordsEmptyDispatchRun(t *testing.T) {
	registry := metrics.New(nil)
	repo := &stubOutboxRepo{
		acquire: func(context.Context, int, time.Time, time.Time, string) ([]*model.OutboxEvent, error) {
			return nil, nil
		},
		markPublished: func(context.Context, string, time.Time) error { return nil },
		markFailed:    func(context.Context, string, int, string, time.Time, bool) error { return nil },
	}

	dispatcher := NewOutboxDispatcher(&config.Config{
		MQ: config.MQConfig{
			Enabled:            true,
			BatchSize:          10,
			MaxAttempts:        3,
			LockTimeoutSec:     30,
			OperationTimeoutMs: 100,
			PublishTimeoutMs:   100,
			PollIntervalMs:     1000,
		},
	}, zap.NewNop(), repo, stubMQPublisher{
		publish: func(context.Context, mq.Message) error { return nil },
	}, registry)

	if err := dispatcher.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}

	body := metricsBody(t, registry)
	if !strings.Contains(body, "outbox_dispatch_runs_total{result=\"empty\"} 1") {
		t.Fatal("expected empty dispatch metric to be recorded")
	}
}

func metricsBody(t *testing.T, registry *metrics.Registry) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	registry.Handler().ServeHTTP(recorder, req)
	return recorder.Body.String()
}
