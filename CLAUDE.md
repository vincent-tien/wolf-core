# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

`wolf-core` (`github.com/vincent-tien/wolf-core`) is a **DDD shared kernel library** for building event-driven microservices in Go. It provides domain modeling primitives, CQRS infrastructure, transactional outbox, and broker-agnostic messaging — consumed as a Go module dependency by service repositories.

## Build & Test Commands

```bash
go build ./...                              # compile all packages
go vet ./...                                # static analysis
go test ./... -race -count=1 -shuffle=on    # full test suite with race detector
go test ./... -run TestFoo                  # single test by name
go test ./path/to/pkg -run TestFoo          # single test in specific package
go test ./... -bench=.                      # run benchmarks
go test ./... -coverprofile=cover.out       # coverage report
```

No Makefile, no linter config, no CI pipeline exists yet. `go vet` + `-race` is the minimum quality bar.

## Architecture

### Layer Dependency Rule

```
Domain (entity, aggregate, vo, event, auth, errors, spec, clock, types)
  ↓ imports allowed downward only
Application (cqrs, messenger, uow, tx, messaging, runtime, policy, validator)
  ↓
Infrastructure (infra/*)
```

Domain packages have **zero infrastructure imports**. The `infra/` tree depends on everything above but nothing above imports `infra/`.

### Package Map

**Domain Layer — Pure Go, no framework dependencies:**

| Package | Purpose |
|---------|---------|
| `entity` | Base struct with ID + audit timestamps. Embed in domain entities |
| `aggregate` | Aggregate root base with event collection + optimistic concurrency version |
| `vo` | Value objects: `Money` (integer cents + ISO currency), `PageRequest`/`PageResponse[T]` (cursor pagination) |
| `event` | Domain event interface, `TypeRegistry` for serialization, `Bus` (in-process pub/sub), `Dispatcher` (handler routing) |
| `errors` | Typed `AppError` with codes (NOT_FOUND, CONFLICT, VALIDATION, etc.) and `errors.Is`/`As` support |
| `auth` | `UserClaims` (JWT payload with lazy `Set` for O(1) role/permission checks), `TokenValidator`, `TokenBlacklist`, `Authorizer` interfaces |
| `spec` | Specification pattern with `And`/`Or`/`Not` composition |
| `clock` | `Clock` interface + `RealClock`/`FakeClock` for deterministic time in tests |
| `types` | Generic `Set[T]` with intersection/union operations |

**Application Layer — Orchestration contracts:**

| Package | Purpose |
|---------|---------|
| `cqrs` | Generic `CommandHandler[C,R]` / `QueryHandler[Q,R]` + middleware chain (logging, validation, metrics, caching) |
| `messenger` | Symfony-inspired message bus: envelope/stamp model, sync/async routing, handler registry, middleware chain, transports (memory, outbox) |
| `tx` | `Runner` interface — transaction port. Impl in `infra/db` |
| `uow` | `UnitOfWork` — wraps aggregate save + outbox insert in single transaction. Prevents dual-write |
| `messaging` | Broker-agnostic `Stream` interface + typed `EventStream`/`CommandStream` wrappers |
| `runtime` | `Module` interface — bounded context registration contract (HTTP, gRPC, events, lifecycle) |
| `validator` | Wraps `go-playground/validator` → returns typed `AppError` |

**Infrastructure Layer (`infra/`):**

| Package | Purpose |
|---------|---------|
| `bootstrap` | Composition root: wires config → DB → cache → broker → outbox → servers. Entry point for services |
| `db` | Write/read connection pools, `TxManager`, pool metrics |
| `cache` | `Client` interface with Redis, local (LRU), noop, and singleflight implementations |
| `events` | Broker factory (`inprocess`/`nats`/`rabbitmq`/`kafka`), outbox store+worker+notifier, dead letter, inbox |
| `middleware/http` | Gin middleware chain: auth, RBAC, CORS, rate limiting, tracing, metrics, recovery, security headers, load shedding, request ID |
| `middleware/grpc` | gRPC interceptor chain: auth, RBAC, tracing, metrics, recovery, error mapping, transaction |
| `auth` | JWT service (issue/validate), Redis token blacklist, password hashing (bcrypt) |
| `config` | Viper-based config loading with struct validation |
| `observability` | Zap logger, OpenTelemetry tracing, Prometheus metrics |
| `seed` | Database seeding framework: factories, references, topological sort, truncation |
| `di` | Lightweight DI container |
| `decorator` | Generic decorator chain (cache, logging, metrics) for repository/service wrapping |
| `modular` | Module registry with dependency-ordered startup + `Container` for DI |
| `resilience` | Circuit breaker (sony/gobreaker wrapper) |
| `ratelimit` | Fixed-window and sliding-window rate limiters (Redis-backed) |
| `security` | TLS configuration helpers |
| `worker` | Background worker pool patterns |

### Key Patterns

**Transactional Outbox** — The critical write path: `UnitOfWork.Execute()` saves aggregate state + drains domain events into the outbox table in one DB transaction. The `outbox.Worker` polls/relays events to the configured broker. `outbox.Notifier` uses PostgreSQL LISTEN/NOTIFY to reduce poll latency.

**Module System** — Each bounded context implements `runtime.Module`. Bootstrap calls `RegisterModules()` with a manifest, which topologically sorts by `DependsOn()`, registers event types, mounts HTTP/gRPC routes, and starts lifecycle hooks. Modules receive a `modular.Container` with all platform deps.

**CQRS Middleware** — Handlers are wrapped via `cqrs.ChainCommand(handler, mw1, mw2)`. Built-in middlewares for logging, validation, Prometheus metrics, and cache-aside. All are infrastructure-free (no Gin/gRPC imports).

**Messenger** — Alternative to raw CQRS when you need envelope metadata (stamps), async routing, or transport abstraction. Envelope carries message + stamp chain through middleware pipeline.

### Aggregate Event Flow

```
Command → Handler → Aggregate.Method() → aggregate.AddDomainEvent()
  → UoW.Execute(agg, func(txCtx) { repo.Save(txCtx, agg) })
    → 1. repo.Save succeeds
    → 2. agg.ClearEvents() drains events
    → 3. outbox.Store.Insert(txCtx, events)
    → 4. TX commits atomically
  → outbox.Worker polls → Stream.Publish()
```

## Conventions

- **Go version**: 1.26 (generics used extensively)
- **Error wrapping**: Always `fmt.Errorf("context: %w", err)` — never bare returns
- **Constructor validation**: `New()` functions panic on nil dependencies (fail-fast at startup)
- **Value object immutability**: Operations return new instances; no setters
- **Context propagation**: Every public method takes `context.Context` as first param
- **Package naming**: Single-word lowercase (`vo`, `tx`, `uow`); sub-packages use path nesting
- **Testing**: `testify/assert` + `testify/require`; `go.uber.org/goleak` for goroutine leak detection; table-driven tests; `clock.FakeClock` for time-dependent tests
