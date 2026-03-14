// timeout.go — Messenger middleware enforcing a maximum handler execution duration.
package middleware

import (
	"context"
	"time"

	"github.com/vincent-tien/wolf-core/messenger"
)

// Timeout enforces a maximum execution duration for handler dispatch.
type Timeout struct {
	defaultTimeout time.Duration
	perType        map[string]time.Duration
}

// NewTimeout creates a timeout middleware with a global default.
func NewTimeout(defaultTimeout time.Duration) *Timeout {
	return &Timeout{
		defaultTimeout: defaultTimeout,
		perType:        make(map[string]time.Duration),
	}
}

// WithTypeTimeout sets a timeout for a specific message type.
func (m *Timeout) WithTypeTimeout(msgType string, d time.Duration) *Timeout {
	m.perType[msgType] = d
	return m
}

func (m *Timeout) Handle(ctx context.Context, env messenger.Envelope, next messenger.MiddlewareNext) (messenger.DispatchResult, error) {
	d := m.defaultTimeout
	if perType, ok := m.perType[env.MessageTypeName()]; ok {
		d = perType
	}

	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()

	return next(ctx, env)
}
