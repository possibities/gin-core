package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/possibities/gin-core/pkg/config"
	"go.uber.org/zap"
)

func TestPoolExecutesSubmittedTasks(t *testing.T) {
	pool := NewPool(&config.Config{
		Worker: config.WorkerConfig{
			Workers:         2,
			QueueSize:       8,
			SubmitTimeoutMs: 100,
		},
	}, zap.NewNop())

	var ran atomic.Int32
	if err := pool.Submit(context.Background(), func(context.Context) error {
		ran.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ran.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ran.Load() != 1 {
		t.Fatal("expected submitted task to run")
	}
}

func TestPoolShutdownRejectsNewTasks(t *testing.T) {
	pool := NewPool(&config.Config{
		Worker: config.WorkerConfig{
			Workers:         1,
			QueueSize:       1,
			SubmitTimeoutMs: 50,
		},
	}, zap.NewNop())

	if err := pool.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := pool.Submit(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("expected Submit() after shutdown to fail")
	}
}
