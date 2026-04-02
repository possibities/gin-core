package event

import (
	"context"
	"sync"

	"github.com/possibities/gin-boilerplate/internal/worker"
	pkglogger "github.com/possibities/gin-boilerplate/pkg/logger"
	"go.uber.org/zap"
)

type Message interface {
	Topic() string
}

type Handler func(context.Context, Message) error

type Publisher interface {
	Publish(ctx context.Context, message Message) error
}

type Bus struct {
	logger   *zap.Logger
	workers  *worker.Pool
	mu       sync.RWMutex
	handlers map[string][]Handler
}

func NewBus(logger *zap.Logger, workers *worker.Pool) *Bus {
	return &Bus{
		logger:   logger,
		workers:  workers,
		handlers: make(map[string][]Handler),
	}
}

func (b *Bus) Subscribe(topic string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[topic] = append(b.handlers[topic], handler)
}

func (b *Bus) Publish(ctx context.Context, message Message) error {
	b.mu.RLock()
	handlers := append([]Handler(nil), b.handlers[message.Topic()]...)
	b.mu.RUnlock()

	for _, handler := range handlers {
		handler := handler
		detached := detachContext(ctx, b.logger)
		if err := b.workers.Submit(ctx, func(context.Context) error {
			return handler(detached, message)
		}); err != nil {
			return err
		}
	}
	return nil
}

func detachContext(ctx context.Context, logger *zap.Logger) context.Context {
	traceID := pkglogger.TraceIDFromContext(ctx)
	return pkglogger.WithRequestContext(context.Background(), pkglogger.WithTraceID(logger, traceID), traceID)
}
