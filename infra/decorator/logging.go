// logging.go — CQRS middleware that logs command/query execution with duration.
package decorator

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// WithLogging returns a Middleware that records the operation name, execution
// duration, and any error. Successful calls are logged at Info level;
// failed calls are logged at Warn level with the error field.
func WithLogging[In, Out any](logger *zap.Logger, operation string) Middleware[In, Out] {
	return func(next Func[In, Out]) Func[In, Out] {
		return func(ctx context.Context, in In) (Out, error) {
			start := time.Now()

			result, err := next(ctx, in)

			fields := []zap.Field{
				zap.String("operation", operation),
				zap.Duration("duration", time.Since(start)),
			}

			if err != nil {
				fields = append(fields, zap.Error(err))
				logger.Warn("operation failed", fields...)
				return result, err
			}

			logger.Info("operation succeeded", fields...)
			return result, nil
		}
	}
}
