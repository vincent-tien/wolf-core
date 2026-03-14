// Package runtime defines the lifecycle and registration contracts for
// application modules in the wolf-be platform. Each bounded-context module
// implements the Module interface to participate in the server startup,
// route registration, and graceful shutdown sequences.
package runtime

import (
	"context"

	sharedauth "github.com/vincent-tien/wolf-core/auth"
	"github.com/vincent-tien/wolf-core/event"
	"github.com/vincent-tien/wolf-core/messaging"
)

// Module is the lifecycle and registration contract every bounded-context
// module must satisfy. Modules are composed at the application entry point
// and driven by the server runtime.
//
// router and server are typed as interface{} to avoid importing transport
// framework packages (gin, grpc) into the shared kernel. The application
// entry point performs the concrete type assertions.
type Module interface {
	// Name returns the unique module identifier used in logs and metrics.
	Name() string

	// RegisterEvents registers the module's domain event payload types with
	// the given TypeRegistry. Called during bootstrap before any subscribers
	// are wired so that serialization/deserialization is available.
	RegisterEvents(r *event.TypeRegistry)

	// RegisterHTTP mounts the module's HTTP handlers onto the provided router.
	// router is expected to be a *gin.RouterGroup at runtime.
	RegisterHTTP(router interface{})

	// RegisterGRPC registers the module's gRPC service implementations with
	// the provided server. server is expected to be a *grpc.Server at runtime.
	RegisterGRPC(server interface{})

	// RegisterSubscribers attaches domain event handlers to the provided
	// Subscriber. Called once during application startup, before OnStart.
	RegisterSubscribers(sub event.Subscriber) error

	// OnStart is called after all registrations are complete and the server
	// is ready to accept traffic. Use it to start background goroutines.
	OnStart(ctx context.Context) error

	// OnStop is called during graceful shutdown. Implementations must release
	// resources and stop background goroutines before returning.
	OnStop(ctx context.Context) error
}

// DependencyDeclarer is an optional interface that modules can implement to
// declare ordering dependencies on other modules. Bootstrap uses these
// declarations for topological sorting during startup.
type DependencyDeclarer interface {
	DependsOn() []string
}

// StreamModule is an optional extension that modules can implement to register
// stream-based subscriptions. Bootstrap calls RegisterStreams if the module
// implements this interface.
type StreamModule interface {
	RegisterStreams(stream messaging.Stream) error
}

// HTTPMiddlewareProvider is an optional interface that modules can implement to
// declare middleware applied to the module's entire HTTP route group. Bootstrap
// wraps the module's router in a sub-group with these handlers before calling
// RegisterHTTP. Elements must be gin.HandlerFunc at runtime; []any avoids
// importing gin into the shared kernel.
type HTTPMiddlewareProvider interface {
	HTTPMiddleware() []any
}

// HealthProbeProvider is an optional interface that modules can implement to
// expose named readiness probes. Bootstrap type-asserts each module and
// registers any declared probes with the ReadinessChecker automatically.
type HealthProbeProvider interface {
	HealthChecks() map[string]func(ctx context.Context) error
}

// SessionRevocationProvider is an optional interface that modules can implement
// to supply a durable (DB-backed) session revocation checker. Bootstrap
// type-asserts each module and wires the first provider it finds into the
// platform JWTService. This replaces module-side mutation of platform singletons.
type SessionRevocationProvider interface {
	SessionRevocationChecker() sharedauth.SessionRevocationChecker
}

// GRPCInterceptorProvider is an optional interface that modules can implement
// to declare unary interceptors scoped to the module's gRPC services. Elements
// must be grpc.UnaryServerInterceptor at runtime; []any avoids importing grpc
// into the shared kernel.
type GRPCInterceptorProvider interface {
	GRPCServicePrefix() string
	GRPCInterceptors() []any
}
