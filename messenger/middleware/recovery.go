// recovery.go — Messenger middleware that recovers panics during handler dispatch.
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/vincent-tien/wolf-core/messenger"
)

// Recovery catches panics in downstream handlers and converts them to errors.
// Should be the FIRST middleware in the chain (outermost).
type Recovery struct {
	logger *slog.Logger
}

// NewRecovery creates a recovery middleware.
func NewRecovery(logger *slog.Logger) *Recovery {
	return &Recovery{logger: logger}
}

func (m *Recovery) Handle(ctx context.Context, env messenger.Envelope, next messenger.MiddlewareNext) (result messenger.DispatchResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			m.logger.ErrorContext(ctx, "messenger: panic recovered",
				slog.String("message_type", env.MessageTypeName()),
				slog.String("panic", fmt.Sprint(r)),
				slog.String("stack", string(stack)),
			)
			err = fmt.Errorf("messenger: panic in handler for %s: %v", env.MessageTypeName(), r)
		}
	}()
	return next(ctx, env)
}
