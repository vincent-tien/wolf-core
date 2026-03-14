# Architecture Guide

wolf-core follows **Clean Architecture** with **DDD tactical patterns**, **CQRS**, and **transactional outbox** for reliable event publishing.

---

## Layer Dependency Rule

```
Domain Layer (pure Go — zero infrastructure imports)
    entity, aggregate, vo, event, auth, errors, spec, clock, types, policy
        ↓ imports allowed downward only
Application Layer (orchestration contracts)
    cqrs, messenger, tx, uow, messaging, runtime, validator
        ↓
Infrastructure Layer (concrete implementations)
    infra/*
```

**The cardinal rule:** Nothing in the domain or application layers imports from `infra/`. All infrastructure concerns are injected via interfaces defined in the upper layers.

> **Why this matters:** Domain logic remains testable without databases, brokers, or HTTP frameworks. You can unit-test an aggregate or command handler with zero infrastructure setup.

---

## Key Architectural Patterns

### 1. Aggregate Root + Domain Events

Every state-changing operation happens through an aggregate root. State transitions emit domain events that capture what happened.

```
Command → Handler → Aggregate.Method()
                        ├── Validates invariants
                        ├── Mutates state
                        └── Calls AddDomainEvent()
```

- Aggregates embed `aggregate.Base` for identity, versioning, and event collection
- Events are collected in-memory during the method call, not published immediately
- `ClearEvents()` returns a defensive copy — safe for deferred publishing

### 2. Transactional Outbox

The critical write path that prevents dual-write inconsistency:

```
UnitOfWork.Execute(ctx, aggregate, func(txCtx) {
    repo.Save(txCtx, aggregate)     ← 1. Persist aggregate state
})
    ├── 2. ClearEvents() drains events from aggregate (only if step 1 succeeded)
    ├── 3. outbox.Store.Insert(txCtx, events) writes events to outbox table
    └── 4. Transaction commits atomically (all or nothing)

outbox.Worker polls → Stream.Publish() → broker (NATS/RabbitMQ/Kafka)
```

**Guarantee:** Domain state and outbox events are always consistent. If the transaction rolls back, neither the state change nor the events persist.

**Optimization:** When `outbox.notify_enabled = true`, PostgreSQL `LISTEN/NOTIFY` wakes the worker immediately on INSERT instead of waiting for the next poll tick.

### 3. CQRS (Command Query Responsibility Segregation)

Commands and queries are handled by separate, type-safe handler interfaces:

```go
// Commands mutate state, return a result
CommandHandler[C Command, R any].Handle(ctx, cmd) → (R, error)

// Queries are read-only, return projections
QueryHandler[Q Query, R any].Handle(ctx, query) → (R, error)
```

Cross-cutting concerns are applied via middleware chains:

```
Request → Validation → Metrics → Logging → Cache → Handler
```

Middleware is composed with `ChainCommand(handler, mw1, mw2, mw3)` — last middleware wraps outermost.

### 4. Module System

Each bounded context is a `runtime.Module` that declares its HTTP routes, gRPC services, event subscribers, and lifecycle hooks:

```
bootstrap.New("config.yaml")
    └── app.RegisterModules(manifest)
            ├── For each module:
            │   ├── factory(Container) → Module
            │   ├── RegisterEvents(TypeRegistry)
            │   ├── RegisterSubscribers(EventBus)
            │   ├── RegisterStreams(Stream)     ← if StreamModule
            │   ├── RegisterHTTP(RouterGroup)
            │   └── RegisterGRPC(Server)
            └── Topological sort by DependsOn()
```

Modules receive a `modular.Container` with all platform dependencies (DB pools, cache, auth, etc.) — they never import `bootstrap` directly.

### 5. Broker-Agnostic Messaging

The `messaging.Stream` interface abstracts away the broker:

| Driver | Use Case |
|--------|----------|
| `inprocess` | Development, testing |
| `nats` | JetStream — durable, ordered, replay |
| `rabbitmq` | AMQP — routing, exchanges, queues |
| `kafka` | High-throughput, partitioned, exactly-once |

All drivers implement the same `Publish`/`Subscribe` contract. Typed wrappers (`EventStream`, `CommandStream`) add serialization via the `TypeRegistry`.

---

## Bootstrap Initialization Order

The composition root (`infra/bootstrap`) wires dependencies in strict order:

| Step | Component | Depends On |
|------|-----------|------------|
| 1 | Config (Viper + env vars) | — |
| 2 | Logger (Zap) | Config |
| 3 | Metrics (Prometheus) | — |
| 4 | Tracing (OpenTelemetry) | Config |
| 5 | Write DB pool | Config |
| 6 | Read DB pool | Config |
| 7 | Transaction manager | Write DB |
| 8 | Cache client (Redis/local/noop) | Config |
| 9 | JWT service + token blacklist | Cache, Config |
| 10 | Event bus (in-process) | Logger |
| 11 | Messaging stream (broker) | Config, Logger |
| 12 | Outbox store + worker | Write DB, Stream |
| 13 | HTTP server + middleware chain | All above |
| 14 | gRPC server + interceptors | All above |
| 15 | Readiness checker | DB, Cache, Broker |

On error during init, all previously created resources are cleaned up in LIFO order via a `closers` stack.

---

## HTTP Middleware Chain

Applied to every request in order:

```
Request ID → Recovery → Tracing → Logging → Metrics → Rate Limit →
Load Shedding → CORS → Security Headers → Timeout → [Auth] → [RBAC] → Handler
```

Auth and RBAC are applied per-route by modules, not globally.

## gRPC Interceptor Chain

```
Recovery → Logging → Tracing → Metrics → [Auth] → [RBAC] → [Transaction] → Handler
```

---

## Configuration

YAML config with automatic environment variable overrides:

```
db.write.dsn        → DB_WRITE_DSN
jwt.secret_key      → JWT_SECRET_KEY
broker.driver       → BROKER_DRIVER
log.level           → LOG_LEVEL
```

If the YAML file doesn't exist, bootstrap falls back to env-var-only mode. See `infra/config/config.go` for the full config struct.

---

## Error Flow

```
Domain error (aggregate/handler)
    → errors.AppError{Code, Message, Field, Err}
        → HTTP middleware maps Code → HTTP status
        → gRPC interceptor maps Code → gRPC status
```

| ErrorCode | HTTP Status | gRPC Code |
|-----------|-------------|-----------|
| NOT_FOUND | 404 | NotFound |
| CONFLICT | 409 | AlreadyExists |
| VALIDATION | 400 | InvalidArgument |
| UNAUTHORIZED | 401 | Unauthenticated |
| FORBIDDEN | 403 | PermissionDenied |
| INTERNAL | 500 | Internal |
| RATE_LIMITED | 429 | ResourceExhausted |
| UNAVAILABLE | 503 | Unavailable |

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| `interface{}` for router/server in Module | Avoids importing Gin/gRPC into the shared kernel |
| Fail-fast `panic` in constructors | Invalid DI wiring is a programming error, not a runtime condition |
| Defensive copy in `ClearEvents()` | Prevents mutation after events are handed to the outbox |
| `tx.Runner` as a port | Repositories use `txCtx` without importing `database/sql` |
| Generic handlers over reflection | Compile-time type safety; no runtime type assertion failures |
| Outbox over direct publish | Guarantees at-least-once delivery without 2PC |
| Optimistic concurrency (version field) | Scales better than pessimistic locks for most DDD workloads |
