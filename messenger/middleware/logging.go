// logging.go — Messenger middleware that logs message dispatch with duration.
package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/vincent-tien/wolf-core/messenger"
)

// Logging logs dispatch start, result, and duration.
type Logging struct {
	logger *slog.Logger
}

// NewLogging creates a logging middleware.
func NewLogging(logger *slog.Logger) *Logging {
	return &Logging{logger: logger}
}

func (m *Logging) Handle(ctx context.Context, env messenger.Envelope, next messenger.MiddlewareNext) (messenger.DispatchResult, error) {
	msgType := env.MessageTypeName()
	start := time.Now()

	result, err := next(ctx, env)
	duration := time.Since(start)

	if err != nil {
		m.logger.ErrorContext(ctx, "messenger: dispatch failed",
			slog.String("message_type", msgType),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
		)
	} else {
		level := slog.LevelInfo
		attrs := []slog.Attr{
			slog.String("message_type", msgType),
			slog.Duration("duration", duration),
			slog.Bool("async", result.Async),
		}
		m.logger.LogAttrs(ctx, level, "messenger: dispatch complete", attrs...)
	}

	return result, err
}
