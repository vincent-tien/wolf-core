# Glossary

DDD, CQRS, and event-driven terms as used in wolf-core.

---

## Domain-Driven Design (DDD)

### Aggregate

A cluster of domain objects treated as a single unit for state changes. Has one **aggregate root** that enforces invariants and controls access. In wolf-core, aggregates embed `aggregate.Base`.

> **wolf-core:** Every aggregate must go through `UnitOfWork.Execute()` to persist state and publish events atomically.

### Aggregate Root

The top-level entity in an aggregate. External code can only reference the root — never internal entities. The root ensures the entire aggregate is in a valid state.

### Bounded Context

A logical boundary within which a domain model is defined and consistent. Each bounded context has its own ubiquitous language and may model the same real-world concept differently. In wolf-core, each `runtime.Module` represents one bounded context.

### Domain Event

An immutable record of something that happened in the domain. Named in past tense: `order.placed`, `payment.captured`. Carries a payload, metadata (trace/correlation IDs), and aggregate identity.

> **wolf-core:** Events implement the `event.Event` interface. Payload types must be registered in `event.TypeRegistry` for serialization.

### Entity

An object with a distinct identity that persists over time. Two entities are equal if they have the same ID, regardless of other attribute values. In wolf-core, entities embed `entity.Base`.

### Invariant

A business rule that must always be true. Aggregates enforce invariants — they reject operations that would violate them. Example: "An order total cannot be negative."

### Repository

An interface that provides access to aggregates. Defined in the domain layer (port), implemented in infrastructure (adapter). Repositories work with whole aggregates, not arbitrary SQL queries.

### Shared Kernel

A subset of the domain model that is shared between bounded contexts. wolf-core itself is a shared kernel — it provides common domain building blocks (aggregates, events, errors) used by all service modules.

### Specification

A predicate object that determines whether a domain object satisfies some criteria. Specifications can be composed with `And`, `Or`, `Not`. In wolf-core, the `spec` package provides generic `Spec[T]`.

### Ubiquitous Language

The shared vocabulary between developers and domain experts within a bounded context. Code should use the same terms as the business. If the business says "place an order," the code should have `PlaceOrder`, not `CreateOrder`.

### Value Object

An immutable object defined entirely by its attributes, with no distinct identity. Two value objects with the same attributes are equal. In wolf-core, `vo.Money` is a value object: `NewMoney(4999, "USD")`.

---

## CQRS (Command Query Responsibility Segregation)

### Command

A message expressing intent to change system state. Handled exactly once. In wolf-core, commands implement the `cqrs.Command` marker interface (empty interface — any struct qualifies).

> **Example:** `PlaceOrderCmd{CustomerID: "...", Total: 4999}`

### Query

A message requesting data without side effects. May be cached, retried, or handled multiple times. In wolf-core, queries implement the `cqrs.Query` marker interface.

> **Example:** `GetOrderQuery{ID: "..."}`

### Command Handler

Processes a command: validates input, enforces business rules, persists state, emits events. In wolf-core: `cqrs.CommandHandler[C, R]` with `Handle(ctx, cmd) → (R, error)`.

### Query Handler

Reads from the data store and returns a projection/DTO. Never mutates state. In wolf-core: `cqrs.QueryHandler[Q, R]` with `Handle(ctx, query) → (R, error)`.

### Middleware

A function that wraps a handler to add cross-cutting behavior (logging, validation, metrics, caching). Applied via `cqrs.ChainCommand()` or `cqrs.ChainQuery()`.

### Void

The zero-size return type (`struct{}`) for commands that produce no result. Use `cqrs.AsVoidCommand()` to adapt void-returning functions into the middleware chain.

---

## Event-Driven Architecture

### Event Bus

In-process publish/subscribe mechanism. Publishers emit events; subscribers react. In wolf-core, `event.Bus` is used for intra-service communication (same process).

### Event Stream

Durable, broker-backed publish/subscribe for cross-service communication. In wolf-core, `messaging.Stream` abstracts NATS, RabbitMQ, and Kafka behind a common interface.

### Transactional Outbox

A pattern that ensures domain state changes and event publication happen atomically. Events are written to an outbox table in the same database transaction as the aggregate state change. A relay worker polls the table and publishes to the broker.

> **wolf-core:** `uow.UnitOfWork` implements this pattern. `infra/events/outbox` provides the store, worker, and LISTEN/NOTIFY notifier.

### Dead Letter Queue (DLQ)

Where events go after exhausting retry attempts. Prevents one bad event from blocking the entire pipeline. In wolf-core, `infra/events/deadletter` stores failed events for manual inspection.

### Idempotency

The property that processing the same message multiple times produces the same result as processing it once. Critical for at-least-once delivery guarantees. Consumers must be idempotent — use upserts, deduplication keys, or the inbox pattern.

### Inbox Pattern

The consumer-side counterpart to the outbox pattern. Incoming message IDs are stored in an inbox table with a unique constraint, preventing duplicate processing. In wolf-core, `infra/events/inbox` provides this.

### Correlation ID

A unique identifier that traces a request across multiple services. Passed in event metadata so you can reconstruct the full chain of events triggered by a single user action.

### Causation ID

The ID of the event that caused this event to be produced. Combined with correlation ID, enables building a full causal tree of events.

---

## Infrastructure

### Bootstrap

The composition root that wires all platform dependencies and starts all servers. In wolf-core, `infra/bootstrap.App` owns the construction order, readiness registration, and lifecycle management.

### Module

A self-contained bounded context that registers its HTTP routes, gRPC services, event subscribers, and lifecycle hooks with the platform. Implements `runtime.Module`.

### Container

A typed dependency bag (`modular.Container`) that module factories receive. Provides access to all platform dependencies (DB, cache, auth, etc.) without importing bootstrap internals.

### Circuit Breaker

A resilience pattern that stops calling a failing dependency after repeated failures, allowing it time to recover. States: closed (normal) → open (failing, fast-reject) → half-open (testing recovery). In wolf-core, `infra/resilience` wraps `gobreaker`.

### Unit of Work

A pattern that tracks changes to aggregates and coordinates writing those changes as a single transaction. In wolf-core, `uow.UnitOfWork` combines aggregate persistence with outbox event insertion.

### Optimistic Concurrency

A strategy where the aggregate's version number is checked during save. If another transaction incremented the version since the aggregate was loaded, the save fails with a conflict error rather than silently overwriting changes.

---

## Auth

### UserClaims

The decoded content of a JWT access token: user ID, email, roles, permissions, session ID, expiry. In wolf-core, `auth.UserClaims` provides O(1) `HasRole()` and `HasPermission()` via lazy-initialized sets.

### Token Blacklist

A store of revoked token IDs (JTIs). Checked on every authenticated request. In wolf-core, backed by Redis (`infra/auth.RedisBlacklist`) or a noop for development.

### RBAC (Role-Based Access Control)

Authorization model where permissions are assigned to roles, and roles are assigned to users. In wolf-core, `auth.Authorizer` checks both role and permission requirements.
