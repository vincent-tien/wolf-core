// options.go — Functional options for the outbox transport.
package outbox

import "log/slog"

// Option configures the outbox transport.
type Option func(*Transport)

// WithBatchSize sets the max entries returned per Get call.
func WithBatchSize(n int) Option {
	return func(t *Transport) { t.batchSize = n }
}

// WithLogger sets the transport logger.
func WithLogger(l *slog.Logger) Option {
	return func(t *Transport) { t.logger = l }
}
