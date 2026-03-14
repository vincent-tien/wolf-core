// connection.go — Reconnecting transport connection with exponential backoff.
package worker

import (
	"fmt"
	"math"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// AMQPConnection wraps an AMQP connection with reconnect logic
type AMQPConnection struct {
	conn       *amqp.Connection
	uri        string
	maxRetries int
	logger     *zap.Logger
}

// NewAMQPConnection creates a new AMQP connection with retry and exponential backoff
func NewAMQPConnection(uri string, maxRetries int, logger *zap.Logger) (*AMQPConnection, error) {
	if maxRetries <= 0 {
		maxRetries = 5
	}

	ac := &AMQPConnection{
		uri:        uri,
		maxRetries: maxRetries,
		logger:     logger,
	}

	if err := ac.connect(); err != nil {
		return nil, fmt.Errorf("amqp connect: %w", err)
	}

	return ac, nil
}

func (ac *AMQPConnection) connect() error {
	var lastErr error
	for i := 0; i < ac.maxRetries; i++ {
		conn, err := amqp.Dial(ac.uri)
		if err == nil {
			ac.conn = conn
			ac.logger.Info("amqp connected", zap.String("uri", ac.uri))
			return nil
		}
		lastErr = err
		backoff := time.Duration(math.Pow(2, float64(i))) * time.Second
		ac.logger.Warn("amqp connection attempt failed",
			zap.Int("attempt", i+1),
			zap.Int("max_retries", ac.maxRetries),
			zap.Error(err),
			zap.Duration("backoff", backoff),
		)
		time.Sleep(backoff)
	}
	return fmt.Errorf("failed after %d attempts: %w", ac.maxRetries, lastErr)
}

// Channel opens a new AMQP channel
func (ac *AMQPConnection) Channel() (*amqp.Channel, error) {
	if ac.conn == nil || ac.conn.IsClosed() {
		if err := ac.connect(); err != nil {
			return nil, err
		}
	}
	return ac.conn.Channel()
}

// Close closes the underlying connection
func (ac *AMQPConnection) Close() error {
	if ac.conn != nil && !ac.conn.IsClosed() {
		return ac.conn.Close()
	}
	return nil
}
