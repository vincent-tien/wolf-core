# wolf-core

A **DDD shared kernel library** for building event-driven microservices in Go.

Provides domain modeling primitives, CQRS infrastructure, transactional outbox, broker-agnostic messaging, and a module system for composing bounded contexts — consumed as a Go module dependency by service repositories.

```
go get github.com/vincent-tien/wolf-core
```

> **Go 1.26+** required. Generics used extensively for type-safe handlers, collections, and value objects.

---

## Why wolf-core?

Building microservices with DDD/CQRS/Event Sourcing involves significant boilerplate: aggregate roots, domain events, transactional outbox, CQRS middleware chains, module lifecycle, and cross-cutting infrastructure. wolf-core extracts these into a shared kernel so service teams focus on domain logic, not plumbing.

**What you get:**

| Concern | wolf-core provides |
|---------|--------------------|
| Domain modeling | `aggregate.Base`, `entity.Base`, `vo.Money`, `spec.Spec[T]` |
| Domain events | `event.Event` interface, `TypeRegistry`, in-process `Bus`, `Dispatcher` |
| CQRS | Generic `CommandHandler[C,R]` / `QueryHandler[Q,R]` + middleware chain |
| Transactional outbox | `uow.UnitOfWork` — atomic aggregate save + outbox insert |
| Messaging | Broker-agnostic `Stream` interface (NATS, RabbitMQ, Kafka, in-process) |
| Module system | `runtime.Module` — bounded context lifecycle (HTTP, gRPC, events) |
| Auth | `UserClaims` with O(1) role/perm checks, JWT service, RBAC middleware |
| Error handling | Typed `AppError` with codes, `errors.Is`/`As` support, HTTP status mapping |
| Infrastructure | Bootstrap composition root, config, cache, observability, middleware chains |

---

## Quick Start

### 1. Define an Aggregate

```go
type Order struct {
    aggregate.Base
    customerID string
    status     string
    total      vo.Money
}

func NewOrder(id, customerID string, total vo.Money) *Order {
    o := &Order{
        Base:       aggregate.NewBase(id, "Order"),
        customerID: customerID,
        status:     "pending",
        total:      total,
    }
    o.AddDomainEvent("order.created", OrderCreatedPayload{
        OrderID:    id,
        CustomerID: customerID,
        Total:      total.Amount(),
        Currency:   total.Currency(),
    })
    return o
}
```

### 2. Write a Command Handler

```go
type CreateOrderCmd struct {
    CustomerID string
    Amount     int64
    Currency   string
}

type CreateOrderHandler struct {
    repo OrderRepository
    uow  *uow.UnitOfWork
}

func (h *CreateOrderHandler) Handle(ctx context.Context, cmd CreateOrderCmd) (string, error) {
    total, err := vo.NewMoney(cmd.Amount, cmd.Currency)
    if err != nil {
        return "", err
    }

    order := NewOrder(uuid.NewString(), cmd.CustomerID, total)

    return order.ID(), h.uow.Execute(ctx, order, func(txCtx context.Context) error {
        return h.repo.Save(txCtx, order)
    })
}
```

### 3. Wire with Middleware

```go
handler := cqrs.ChainCommand[CreateOrderCmd, string](
    &CreateOrderHandler{repo: repo, uow: unitOfWork},
    cqrs.WithCommandValidation[CreateOrderCmd, string](validator.Validate),
    cqrs.WithCommandLogging[CreateOrderCmd, string](logger),
    cqrs.WithCommandMetrics[CreateOrderCmd, string](metrics),
)
```

### 4. Create a Module

```go
type OrderModule struct {
    handler cqrs.CommandHandler[CreateOrderCmd, string]
}

func (m *OrderModule) Name() string                                  { return "orders" }
func (m *OrderModule) RegisterEvents(r *event.TypeRegistry)          { /* register payload types */ }
func (m *OrderModule) RegisterHTTP(router interface{})               { /* mount Gin routes */ }
func (m *OrderModule) RegisterGRPC(server interface{})               { /* register gRPC services */ }
func (m *OrderModule) RegisterSubscribers(sub event.Subscriber) error { return nil }
func (m *OrderModule) OnStart(ctx context.Context) error             { return nil }
func (m *OrderModule) OnStop(ctx context.Context) error              { return nil }
```

### 5. Bootstrap

```go
func main() {
    app, err := bootstrap.New("config.yaml")
    if err != nil {
        log.Fatal(err)
    }

    err = app.RegisterModules([]modular.CatalogEntry{
        {Name: "orders", Factory: NewOrderModule},
    })
    if err != nil {
        log.Fatal(err)
    }

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    if err := app.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

---

## Package Overview

### Domain Layer (no infrastructure imports)

| Package | Purpose |
|---------|---------|
| [`aggregate`](./aggregate) | Aggregate root base with event collection + optimistic concurrency |
| [`entity`](./entity) | Base entity with ID and audit timestamps |
| [`vo`](./vo) | Value objects: `Money`, `PageRequest`/`PageResponse[T]` |
| [`event`](./event) | Domain event interface, type registry, in-process bus, dispatcher |
| [`errors`](./errors) | Typed `AppError` hierarchy with error codes and predicates |
| [`auth`](./auth) | `UserClaims`, token validation, RBAC interfaces |
| [`spec`](./spec) | Specification pattern with `And`/`Or`/`Not` composition |
| [`clock`](./clock) | `Clock` interface with real and fake implementations |
| [`types`](./types) | Generic `Set[T]` and collection utilities |

### Application Layer

| Package | Purpose |
|---------|---------|
| [`cqrs`](./cqrs) | Command/query handlers + middleware chain |
| [`messenger`](./messenger) | Envelope-based message bus with stamps and transports |
| [`tx`](./tx) | Transaction runner port (interface only) |
| [`uow`](./uow) | Unit of Work — atomic aggregate + outbox in one transaction |
| [`messaging`](./messaging) | Broker-agnostic stream interface + typed wrappers |
| [`runtime`](./runtime) | Module lifecycle contract for bounded contexts |
| [`validator`](./validator) | Input validation wrapper returning typed errors |
| [`policy`](./policy) | Authorization policy interface |

### Infrastructure Layer (`infra/`)

| Package | Purpose |
|---------|---------|
| [`infra/bootstrap`](./infra/bootstrap) | Composition root: config → DB → cache → broker → servers |
| [`infra/db`](./infra/db) | Write/read connection pools, transaction manager |
| [`infra/cache`](./infra/cache) | Redis, local LRU, noop, and singleflight cache |
| [`infra/events`](./infra/events) | Broker factory, outbox store/worker/notifier, dead letter, inbox |
| [`infra/middleware`](./infra/middleware) | HTTP (Gin) and gRPC interceptor chains |
| [`infra/auth`](./infra/auth) | JWT service, Redis token blacklist, password hashing |
| [`infra/config`](./infra/config) | Viper-based YAML config with env var overrides |
| [`infra/observability`](./infra/observability) | Zap logging, OpenTelemetry tracing, Prometheus metrics |
| [`infra/modular`](./infra/modular) | Module registry with dependency-ordered startup |
| [`infra/seed`](./infra/seed) | Database seeding framework |
| [`infra/resilience`](./infra/resilience) | Circuit breaker wrapper |
| [`infra/ratelimit`](./infra/ratelimit) | Fixed-window and sliding-window rate limiters |

---

## Development

```bash
go build ./...                              # compile
go vet ./...                                # static analysis
go test ./... -race -count=1 -shuffle=on    # full test suite
go test ./path/to/pkg -run TestFoo          # single test
go test ./... -bench=.                      # benchmarks
```

---

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture Guide](./docs/architecture.md) | Layer rules, patterns, data flow |
| [Getting Started](./docs/getting-started.md) | Step-by-step integration tutorial |
| [Code Structure](./docs/code-structure.md) | Package map and responsibilities |
| [Developer Guide](./docs/developer-guide.md) | Workflow, testing, adding features |
| [Conventions](./docs/conventions.md) | Naming, errors, concurrency, testing |
| [Contributing](./docs/contributing.md) | PR process and quality gates |
| [Troubleshooting](./docs/troubleshooting.md) | Common errors and FAQ |
| [Glossary](./docs/glossary.md) | DDD/CQRS/EDA terminology |

---

## License

See [LICENSE](./LICENSE) for details.
