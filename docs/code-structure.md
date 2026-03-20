# Code Structure

Complete package map for wolf-core with responsibilities, key types, and dependency rules.

---

## Top-Level Layout

```
wolf-core/
├── aggregate/       # Aggregate root base
├── auth/            # Authentication/authorization contracts
├── clock/           # Time abstraction for testing
├── cqrs/            # Command/query handlers + middleware
├── entity/          # Base entity with ID + timestamps
├── errors/          # Typed application error hierarchy
├── event/           # Domain event system
├── infra/           # Infrastructure implementations (see below)
├── messaging/       # Broker-agnostic stream contracts
├── messenger/       # Envelope-based message bus
├── policy/          # Authorization policy interface
├── runtime/         # Module lifecycle contract
├── spec/            # Specification pattern
├── tx/              # Transaction runner port
├── types/           # Generic collections (Set, Map, Filter)
├── uow/             # Unit of Work pattern
├── validator/        # Input validation wrapper
└── vo/              # Value objects (Money, Pagination)
```

---

## Domain Layer Packages

These packages have **zero infrastructure imports** — pure Go only.

### `aggregate`

Aggregate root base with event collection and optimistic concurrency.

| Type | Purpose |
|------|---------|
| `Base` | Embed in domain aggregates. Tracks ID, version, events, timestamps |
| `NewBase(id, type)` | Constructor with pre-allocated event slice (cap 4) |

Key methods: `AddDomainEvent()`, `ClearEvents()`, `IncrementVersion()`, `SetVersion()`

### `entity`

Simple base for entities that aren't aggregate roots.

| Type | Purpose |
|------|---------|
| `Base` | ID + CreatedAt + UpdatedAt. Embed in domain entities |

### `vo` (Value Objects)

Immutable domain value types. Operations return new instances.

| Type | Purpose |
|------|---------|
| `Money` | Integer cents + ISO 4217 currency. `Add()`, `Subtract()`, `Multiply()` |
| `PageRequest` | Cursor-based pagination input (cursor + limit, max 100) |
| `PageResponse[T]` | Generic paginated result with `Items`, `NextCursor`, `HasMore`, `TotalCount` |
| `MapPage[T,U]()` | Transform `PageResponse[T]` to `PageResponse[U]` |

### `event`

Domain event system with multiple sub-concerns.

| Type | Purpose |
|------|---------|
| `Event` | Interface: EventID, EventType, AggregateID, Payload, Metadata |
| `NewEvent()` | Constructor with functional options (`WithCorrelationID`, `WithSource`, etc.) |
| `TypeRegistry` | Maps event type strings to payload structs for serialization |
| `Bus` | In-process pub/sub (Publisher + Subscriber + Close) |
| `Dispatcher` | Routes events to typed handler chains |
| `Metadata` | TraceID, CorrelationID, CausationID, Source |

### `errors`

Typed error hierarchy with HTTP/gRPC status mapping.

| Type | Purpose |
|------|---------|
| `AppError` | Code + Message + Field + wrapped Err. Implements `error`, `Unwrap()`, `Is()` |
| `ErrorCode` | String enum: NOT_FOUND, CONFLICT, VALIDATION, UNAUTHORIZED, FORBIDDEN, INTERNAL, RATE_LIMITED, UNAVAILABLE |
| `NewNotFound()`, `NewConflict()`, ... | Constructors per error code |
| `IsNotFound()`, `IsConflict()`, ... | Predicate helpers using `errors.Is()` |

### `auth`

Authentication and authorization contracts (interfaces only).

| Type | Purpose |
|------|---------|
| `UserClaims` | JWT payload with lazy `Set` for O(1) `HasRole()` / `HasPermission()` |
| `TokenValidator` | Interface: `ValidateAccessToken(ctx, token) → (*UserClaims, error)` |
| `TokenBlacklist` | Interface: `IsBlacklisted()`, `Blacklist()` |
| `SessionRevocationChecker` | Interface: `IsSessionRevoked()` |
| `Permission` | String type with `"resource:action"` format |
| `Authorizer` | Interface: `Authorize(ctx, claims, roles, perms)` |

### `spec`

Specification pattern for composable business rules.

| Type | Purpose |
|------|---------|
| `Spec[T]` | Interface: `Name()`, `IsSatisfiedBy(T) bool` |
| `And()`, `Or()`, `Not()` | Composers returning new `Spec[T]` |
| `Check()`, `CheckAll()` | Run specs and collect violations |

### `clock`

Time abstraction for deterministic testing.

| Type | Purpose |
|------|---------|
| `Clock` | Interface: `Now() time.Time` |
| `RealClock` | Returns `time.Now()` |
| `FakeClock` | Returns a fixed `time.Time` you control |

### `types`

Generic collection utilities.

| Type | Purpose |
|------|---------|
| `Set[T]` | O(1) membership: `Add`, `Contains`, `ContainsAll`, `ContainsAny`, `Intersect`, `Union` |
| `Map()`, `Filter()`, `Reduce()`, `GroupBy()`, `Contains()` | Slice utility functions |

---

## Application Layer Packages

Orchestration contracts — may import domain packages but never `infra/`.

### `cqrs`

Generic command/query handlers with middleware chains.

| Type | Purpose |
|------|---------|
| `Command`, `Query` | Marker interfaces |
| `CommandHandler[C,R]` | `Handle(ctx, cmd) → (R, error)` |
| `QueryHandler[Q,R]` | `Handle(ctx, query) → (R, error)` |
| `CommandHandlerFunc[C,R]` | Function adapter |
| `Void` | Zero-size return type for side-effect-only commands |
| `AsVoidCommand()` | Adapts `func(ctx, C) error` into `CommandHandler[C, Void]` |
| `ChainCommand()`, `ChainQuery()` | Middleware composition |
| `WithCommandLogging()` | Logs command name, duration, error |
| `WithCommandValidation()` | Pre-handler validation |
| `WithCommandMetrics()` | Prometheus histogram + counter |
| `WithCommandCacheInvalidation()` | Cache eviction after success |
| `WithQueryCaching()` | Cache-aside pattern |

### `messenger`

Symfony-inspired message bus with envelope/stamp model.

```
messenger/
├── bus.go           # Bus interface: Dispatch + Query
├── envelope.go      # Envelope = Message + Stamps
├── stamp/           # Metadata stamps (transport, result, etc.)
├── handler/         # Handler resolution
├── router/          # Sync/async transport routing
├── chain/           # Middleware chain builder
├── serde/           # JSON serialization
├── middleware/       # Built-in middleware (tracing, conditional, etc.)
├── transport/       # Transport implementations (memory, outbox)
└── worker/          # Background message consumer
```

### `tx`

Transaction runner port — 1 interface, 0 implementations (those live in `infra/db`).

| Type | Purpose |
|------|---------|
| `Runner` | `RunInTx(ctx, func(txCtx) error) error` |

### `uow`

Unit of Work — atomic aggregate persistence + outbox event insertion.

| Type | Purpose |
|------|---------|
| `UnitOfWork` | `Execute(ctx, agg, fn)` and `ExecuteMulti(ctx, aggs, fn)` |
| `Aggregate` | Interface constraint: `ClearEvents() []event.Event` |
| `OutboxInserter` | Port for outbox writes |

### `messaging`

Broker-agnostic stream contracts and typed wrappers.

| Type | Purpose |
|------|---------|
| `Message` | Interface: ID, Subject, Data, Headers, Ack/Nak |
| `Stream` | Interface: Publish, Subscribe, Close |
| `EventStream` | Typed event pub/sub using TypeRegistry |
| `CommandStream` | Typed command pub/sub with reply |
| `SubscribeConfig` | Consumer group, durable, retry, ack policies |

### `runtime`

Module lifecycle contract for bounded contexts.

| Type | Purpose |
|------|---------|
| `Module` | Name, RegisterEvents, RegisterHTTP, RegisterGRPC, RegisterSubscribers, OnStart, OnStop |
| `DependencyDeclarer` | Optional: `DependsOn() []string` for boot ordering |
| `StreamModule` | Optional: `RegisterStreams(Stream)` |
| `HTTPMiddlewareProvider` | Optional: module-scoped HTTP middleware |
| `HealthProbeProvider` | Optional: readiness probe registration |
| `SessionRevocationProvider` | Optional: DB-backed session revocation |
| `GRPCInterceptorProvider` | Optional: module-scoped gRPC interceptors |

---

## Infrastructure Layer (`infra/`)

Concrete implementations. Imports from domain and application layers.

### Core Infrastructure

| Package | Purpose | Key Types |
|---------|---------|-----------|
| `infra/bootstrap` | Composition root — wires everything | `App`, `New()`, `RegisterModules()`, `Run()` |
| `infra/modular` | Module registry + DI container | `Container`, `CatalogEntry`, `Registry` |
| `infra/config` | YAML + env var config loading | `Config`, `Load()`, all `*Config` structs |
| `infra/db` | Connection pools + tx manager | `NewWritePool()`, `NewReadPool()`, `TxManager`, `TxRunner` |
| `infra/di` | Lightweight dependency injection | `Container` |

### Middleware

| Package | Purpose |
|---------|---------|
| `infra/middleware/http` | Gin middleware: auth, RBAC, CORS, rate limit, tracing, metrics, recovery, security headers, load shedding, request ID, timeout, error handler |
| `infra/middleware/grpc` | gRPC interceptors: auth, RBAC, tracing, metrics, recovery, error mapper, transaction, logging |

### Events & Messaging

| Package | Purpose |
|---------|---------|
| `infra/events` | Broker factory (`NewBus`, `NewStream`) |
| `infra/events/outbox` | Outbox store, worker, notifier (LISTEN/NOTIFY), lag checker |
| `infra/events/deadletter` | Dead letter store + migrations |
| `infra/events/inbox` | Inbox deduplication |
| `infra/events/inprocess` | In-process event bus implementation |
| `infra/events/nats` | NATS JetStream adapter |

### Auth & Security

| Package | Purpose |
|---------|---------|
| `infra/auth` | JWT service (HS256/RS256), Redis blacklist, bcrypt password hashing |
| `infra/security` | TLS configuration helpers |

### Observability

| Package | Purpose |
|---------|---------|
| `infra/observability/logging` | Zap logger factory |
| `infra/observability/tracing` | OpenTelemetry OTLP exporter setup |
| `infra/observability/metrics` | Prometheus metrics + runtime gauges |

### Resilience & Performance

| Package | Purpose |
|---------|---------|
| `infra/cache` | Multi-backend cache: Redis, local LRU, noop, singleflight |
| `infra/resilience` | Circuit breaker (gobreaker wrapper) |
| `infra/ratelimit` | Fixed-window and sliding-window rate limiters |
| `infra/pool` | Worker pool patterns |
| `infra/concurrency` | Concurrency utilities |
| `infra/idempotency` | Idempotency key store |

### Other

| Package | Purpose |
|---------|---------|
| `infra/seed` | Database seeding: factories, references, topological sort, truncation |
| `infra/decorator` | Generic decorator chain (cache, logging, metrics) for repositories |
| `infra/http` | HTTP server wrapper, metrics server, readiness checker |
| `infra/grpc` | gRPC server wrapper with reflection; `NewNoop()` for disabled mode |
| `infra/io` | Streaming I/O utilities |
| `infra/profiling` | pprof/fgprof profiling setup |
| `infra/runtime` | GOMAXPROCS tuning (automaxprocs) |
| `infra/validation` | Extended validation rules |
| `infra/worker` | Background worker lifecycle |
