package mq

import (
	"context"
	"time"
)

type Message struct {
	ID        string
	Topic     string
	Payload   []byte
	Headers   map[string]string
	Timestamp time.Time
}

type Publisher interface {
	Publish(ctx context.Context, message Message) error
}

type nopPublisher struct{}

func (nopPublisher) Publish(context.Context, Message) error {
	return nil
}
