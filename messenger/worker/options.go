// options.go — Configuration options for the messenger worker.
package worker

import (
	"log/slog"
	"time"
)

// Options configures the worker behavior.
type Options struct {
	Concurrency     int
	PollInterval    time.Duration
	ShutdownTimeout time.Duration
	Logger          *slog.Logger
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{
		Concurrency:     1,
		PollInterval:    time.Second,
		ShutdownTimeout: 30 * time.Second,
		Logger:          slog.Default(),
	}
}

// Option configures a worker.
type Option func(*Options)

// WithConcurrency sets parallel processors per transport.
func WithConcurrency(n int) Option {
	return func(o *Options) { o.Concurrency = n }
}

// WithPollInterval sets the sleep duration when no messages are available.
func WithPollInterval(d time.Duration) Option {
	return func(o *Options) { o.PollInterval = d }
}

// WithShutdownTimeout sets the graceful shutdown deadline.
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *Options) { o.ShutdownTimeout = d }
}

// WithLogger sets the worker logger.
func WithLogger(logger *slog.Logger) Option {
	return func(o *Options) { o.Logger = logger }
}
