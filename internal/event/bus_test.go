package event

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/possibities/gin-boilerplate/internal/worker"
	"github.com/possibities/gin-boilerplate/pkg/config"
	"go.uber.org/zap"
)

type testMessage struct {
	topic string
}

func (m testMessage) Topic() string {
	return m.topic
}

func TestBusDispatchesToSubscriber(t *testing.T) {
	pool := worker.NewPool(&config.Config{
		Worker: config.WorkerConfig{
			Workers:         1,
			QueueSize:       4,
			SubmitTimeoutMs: 100,
		},
	}, zap.NewNop())
	t.Cleanup(func() {
		_ = pool.Shutdown(context.Background())
	})

	bus := NewBus(zap.NewNop(), pool)

	var handled atomic.Int32
	bus.Subscribe("user.updated", func(context.Context, Message) error {
		handled.Add(1)
		return nil
	})

	if err := bus.Publish(context.Background(), testMessage{topic: "user.updated"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if handled.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if handled.Load() != 1 {
		t.Fatal("expected subscriber to run")
	}
}
