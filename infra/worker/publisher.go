// publisher.go — AMQP task publisher with durable topic exchange and Task struct.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Task represents a unit of work to be published
type Task struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Payload   []byte    `json:"payload"`
	Retries   int       `json:"retries"`
	CreatedAt time.Time `json:"created_at"`
}

// NewTask creates a new Task with a generated ID and timestamp
func NewTask(taskType string, payload []byte) Task {
	return Task{
		ID:        uuid.New().String(),
		Type:      taskType,
		Payload:   payload,
		Retries:   0,
		CreatedAt: time.Now().UTC(),
	}
}

// TaskPublisher publishes tasks to an AMQP exchange
type TaskPublisher struct {
	conn     *AMQPConnection
	exchange string
}

// NewTaskPublisher creates a new TaskPublisher
func NewTaskPublisher(conn *AMQPConnection, exchange string) (*TaskPublisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	// Declare a durable topic exchange
	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		return nil, fmt.Errorf("declare exchange: %w", err)
	}

	return &TaskPublisher{conn: conn, exchange: exchange}, nil
}

// Publish sends a task to the exchange with the given routing key
func (p *TaskPublisher) Publish(ctx context.Context, routingKey string, task Task) error {
	ch, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	body, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	return ch.PublishWithContext(ctx, p.exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    task.ID,
		Timestamp:    task.CreatedAt,
		Body:         body,
	})
}
