// consumer.go — AMQP task consumer with concurrent workers and graceful shutdown.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// TaskHandler processes a single task
type TaskHandler func(ctx context.Context, task Task) error

// Consumer consumes tasks from an AMQP queue with concurrent workers
type Consumer struct {
	conn       *AMQPConnection
	queue      string
	exchange   string
	routingKey string
	prefetch   int
	workers    int
	handler    TaskHandler
	logger     *zap.Logger
}

// ConsumerConfig holds consumer configuration
type ConsumerConfig struct {
	Queue      string
	Exchange   string
	RoutingKey string
	Prefetch   int // QoS prefetch count per worker
	Workers    int // Number of concurrent worker goroutines
}

// NewConsumer creates a new Consumer
func NewConsumer(conn *AMQPConnection, cfg ConsumerConfig, handler TaskHandler, logger *zap.Logger) (*Consumer, error) {
	if cfg.Prefetch <= 0 {
		cfg.Prefetch = 10
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	// Declare the queue as durable
	_, err = ch.QueueDeclare(cfg.Queue, true, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("declare queue: %w", err)
	}

	// Bind queue to exchange
	if cfg.Exchange != "" {
		if err := ch.QueueBind(cfg.Queue, cfg.RoutingKey, cfg.Exchange, false, nil); err != nil {
			return nil, fmt.Errorf("bind queue: %w", err)
		}
	}

	return &Consumer{
		conn:       conn,
		queue:      cfg.Queue,
		exchange:   cfg.Exchange,
		routingKey: cfg.RoutingKey,
		prefetch:   cfg.Prefetch,
		workers:    cfg.Workers,
		handler:    handler,
		logger:     logger,
	}, nil
}

// Start begins consuming messages. Blocks until ctx is cancelled.
func (c *Consumer) Start(ctx context.Context) error {
	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	if err = ch.Qos(c.prefetch, 0, false); err != nil {
		return fmt.Errorf("set QoS: %w", err)
	}

	deliveries, err := ch.ConsumeWithContext(ctx, c.queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	c.logger.Info("consumer started", zap.Int("workers", c.workers), zap.String("queue", c.queue))

	var wg sync.WaitGroup
	for i := 0; i < c.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			c.work(ctx, workerID, deliveries)
		}(i)
	}

	<-ctx.Done()
	c.logger.Info("consumer shutting down", zap.String("queue", c.queue))
	wg.Wait()

	return nil
}

func (c *Consumer) work(ctx context.Context, id int, deliveries <-chan amqp.Delivery) {
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}
			c.process(ctx, id, d)
		}
	}
}

func (c *Consumer) process(ctx context.Context, workerID int, d amqp.Delivery) {
	var task Task
	if err := json.Unmarshal(d.Body, &task); err != nil {
		c.logger.Warn("consumer unmarshal error", zap.Int("worker_id", workerID), zap.Error(err))
		_ = d.Nack(false, false) // discard malformed messages
		return
	}

	taskCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := c.handler(taskCtx, task); err != nil {
		c.logger.Warn("consumer task failed", zap.Int("worker_id", workerID), zap.String("task_id", task.ID), zap.Error(err))
		_ = d.Nack(false, true) // requeue for retry
		return
	}

	_ = d.Ack(false)
}
