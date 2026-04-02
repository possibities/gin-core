package mq

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/possibities/gin-core/pkg/config"
	pkgtracing "github.com/possibities/gin-core/pkg/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type rabbitMQPublisher struct {
	mu       sync.Mutex
	conn     *amqp.Connection
	channel  *amqp.Channel
	exchange string
}

type tracedPublisher struct {
	next   Publisher
	tracer trace.Tracer
	system string
}

func NewPublisher(cfg *config.Config, tracingProvider *pkgtracing.Provider) (Publisher, func(), error) {
	if !cfg.MQ.Enabled {
		return wrapPublisherTracing(nopPublisher{}, tracingProvider, "noop"), func() {}, nil
	}

	switch strings.ToLower(strings.TrimSpace(cfg.MQ.Driver)) {
	case "rabbitmq":
		publisher, cleanup, err := newRabbitMQPublisher(cfg)
		if err != nil {
			return nil, nil, err
		}
		return wrapPublisherTracing(publisher, tracingProvider, "rabbitmq"), cleanup, nil
	default:
		return nil, nil, fmt.Errorf("unsupported mq.driver: %s", cfg.MQ.Driver)
	}
}

func newRabbitMQPublisher(cfg *config.Config) (Publisher, func(), error) {
	timeout := time.Duration(cfg.MQ.OperationTimeoutMs) * time.Millisecond
	conn, err := amqp.DialConfig(cfg.MQ.URL, amqp.Config{
		Locale: "en_US",
		Dial:   amqp.DefaultDial(timeout),
	})
	if err != nil {
		return nil, nil, err
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	if err := channel.ExchangeDeclare(
		cfg.MQ.Exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, nil, err
	}

	publisher := &rabbitMQPublisher{
		conn:     conn,
		channel:  channel,
		exchange: cfg.MQ.Exchange,
	}

	cleanup := func() {
		_ = channel.Close()
		_ = conn.Close()
	}
	return publisher, cleanup, nil
}

func (p *rabbitMQPublisher) Publish(ctx context.Context, message Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	headers := amqp.Table{}
	for key, value := range message.Headers {
		headers[key] = value
	}

	return p.channel.PublishWithContext(ctx, p.exchange, message.Topic, false, false, amqp.Publishing{
		MessageId:    message.ID,
		Type:         message.Topic,
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    message.Timestamp.UTC(),
		Headers:      headers,
		Body:         message.Payload,
	})
}

func wrapPublisherTracing(next Publisher, provider *pkgtracing.Provider, system string) Publisher {
	return tracedPublisher{
		next:   next,
		tracer: provider.Tracer("mq.publisher"),
		system: system,
	}
}

func (p tracedPublisher) Publish(ctx context.Context, message Message) error {
	ctx, span := p.tracer.Start(ctx, "mq.publish "+message.Topic, trace.WithSpanKind(trace.SpanKindProducer))
	span.SetAttributes(
		attribute.String("messaging.system", p.system),
		attribute.String("messaging.destination.name", message.Topic),
	)
	err := p.next.Publish(ctx, message)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, message.Topic)
	}
	span.End()
	return err
}
