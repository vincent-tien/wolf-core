// conditional.go — Messenger middleware that applies conditionally based on predicates.
package middleware

import (
	"context"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

// Condition determines when a middleware should execute.
type Condition string

const (
	// Always executes the middleware unconditionally.
	Always Condition = "always"
	// OnCommand executes only when the message is a registered command.
	OnCommand Condition = "command"
	// OnQuery executes only when the message is a registered query.
	OnQuery Condition = "query"
	// OnConsumed executes only when the envelope has a ReceivedStamp (worker side).
	OnConsumed Condition = "consumed"
	// OnAsync executes when the envelope has a SentStamp or ReceivedStamp.
	OnAsync Condition = "async"
	// OnSync executes when the envelope has neither SentStamp nor ReceivedStamp.
	OnSync Condition = "sync"
)

// Conditional wraps a middleware and only executes it when the condition is met.
// When the condition is not met, the inner middleware is skipped entirely and
// next() is called directly — ~2ns overhead for the branch check.
type Conditional struct {
	inner    messenger.Middleware
	cond     Condition
	registry *messenger.HandlerRegistry
}

// NewConditional creates a conditional middleware wrapper.
// registry can be nil if cond is not OnCommand/OnQuery.
func NewConditional(inner messenger.Middleware, cond Condition, registry *messenger.HandlerRegistry) *Conditional {
	return &Conditional{inner: inner, cond: cond, registry: registry}
}

// WhenCommand wraps inner to only execute for command handlers.
func WhenCommand(inner messenger.Middleware, registry *messenger.HandlerRegistry) *Conditional {
	return NewConditional(inner, OnCommand, registry)
}

// WhenQuery wraps inner to only execute for query handlers.
func WhenQuery(inner messenger.Middleware, registry *messenger.HandlerRegistry) *Conditional {
	return NewConditional(inner, OnQuery, registry)
}

// WhenConsumed wraps inner to only execute on the worker/consume side.
func WhenConsumed(inner messenger.Middleware) *Conditional {
	return NewConditional(inner, OnConsumed, nil)
}

func (c *Conditional) Handle(ctx context.Context, env messenger.Envelope, next messenger.MiddlewareNext) (messenger.DispatchResult, error) {
	if !c.shouldExecute(env) {
		return next(ctx, env)
	}
	return c.inner.Handle(ctx, env, next)
}

func (c *Conditional) shouldExecute(env messenger.Envelope) bool {
	switch c.cond {
	case Always:
		return true
	case OnCommand:
		isCmd, ok := c.resolveIsCommand(env)
		return ok && isCmd
	case OnQuery:
		isCmd, ok := c.resolveIsCommand(env)
		return ok && !isCmd
	case OnConsumed:
		return env.HasStamp(stamp.NameReceived)
	case OnAsync:
		return env.HasStamp(stamp.NameSent) || env.HasStamp(stamp.NameReceived)
	case OnSync:
		return !env.HasStamp(stamp.NameSent) && !env.HasStamp(stamp.NameReceived)
	default:
		return true
	}
}

func (c *Conditional) resolveIsCommand(env messenger.Envelope) (isCmd bool, ok bool) {
	if c.registry == nil {
		return false, false
	}
	h, err := c.registry.Resolve(env.Message)
	if err != nil {
		return false, false
	}
	return h.IsCommand(), true
}
