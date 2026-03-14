# Getting Started

This guide walks you through integrating wolf-core into a new service and creating your first bounded-context module.

---

## Prerequisites

- **Go 1.26+** installed
- **PostgreSQL** running (for outbox, transactions)
- A message broker running (or use `inprocess` driver for local dev)

---

## Step 1: Create Your Service Repository

```bash
mkdir my-service && cd my-service
go mod init github.com/your-org/my-service
go get github.com/vincent-tien/wolf-core
```

---

## Step 2: Create a Config File

Create `config.yaml` at the project root:

```yaml
app:
  name: my-service
  env: development
  shutdown_timeout: 30s

http:
  port: 8080
  read_timeout: 10s
  write_timeout: 30s

grpc:
  port: 9090

db:
  write:
    dsn: "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
    max_open_conns: 25
    max_idle_conns: 10
    conn_max_lifetime: 5m
  read:
    dsn: "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
    max_open_conns: 25
    max_idle_conns: 10
    conn_max_lifetime: 5m

cache:
  driver: noop       # use "redis" in staging/production

broker:
  driver: inprocess   # use "nats", "rabbitmq", or "kafka" in production

log:
  level: debug
  format: console     # use "json" in production

jwt:
  signing_method: HS256
  secret_key: "your-32-char-secret-key-here!!!!"
  access_token_ttl: 15m
  refresh_token_ttl: 720h
  issuer: my-service

outbox:
  poll_interval: 1s
  batch_size: 100
  max_retries: 5
  retention: 168h
```

> **Tip:** Every config key can be overridden via environment variables. `db.write.dsn` becomes `DB_WRITE_DSN`.

---

## Step 3: Create the Outbox Migration

The transactional outbox requires a database table. Create your first migration:

```sql
-- migrations/001_create_outbox.up.sql

CREATE TABLE IF NOT EXISTS outbox_events (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id        UUID        NOT NULL,
    event_type      TEXT        NOT NULL,
    aggregate_id    TEXT        NOT NULL,
    aggregate_type  TEXT        NOT NULL,
    payload         JSONB       NOT NULL,
    metadata        JSONB       NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ,
    retry_count     INT         NOT NULL DEFAULT 0,
    last_error      TEXT,
    status          TEXT        NOT NULL DEFAULT 'pending'
);

CREATE INDEX idx_outbox_pending ON outbox_events (status, created_at)
    WHERE status = 'pending';
```

---

## Step 4: Define Your Domain

### 4a. Aggregate

```go
// internal/orders/domain/order.go
package domain

import (
    "time"

    "github.com/google/uuid"
    "github.com/vincent-tien/wolf-core/aggregate"
)

type Order struct {
    aggregate.Base
    customerID string
    status     string
    total      int64
    placedAt   time.Time
}

func NewOrder(customerID string, total int64) *Order {
    id := uuid.NewString()
    o := &Order{
        Base:       aggregate.NewBase(id, "Order"),
        customerID: customerID,
        status:     "pending",
        total:      total,
        placedAt:   time.Now().UTC(),
    }
    o.AddDomainEvent("order.placed", OrderPlaced{
        OrderID:    id,
        CustomerID: customerID,
        Total:      total,
    })
    return o
}

func (o *Order) ID() string         { return o.Base.ID() }
func (o *Order) CustomerID() string  { return o.customerID }
func (o *Order) Status() string      { return o.status }
func (o *Order) Total() int64        { return o.total }
```

### 4b. Event Payload

```go
// internal/orders/domain/events.go
package domain

type OrderPlaced struct {
    OrderID    string `json:"order_id"`
    CustomerID string `json:"customer_id"`
    Total      int64  `json:"total"`
}
```

### 4c. Repository Interface (in domain layer)

```go
// internal/orders/domain/repository.go
package domain

import "context"

type OrderRepository interface {
    Save(ctx context.Context, order *Order) error
    FindByID(ctx context.Context, id string) (*Order, error)
}
```

---

## Step 5: Write a Command Handler

```go
// internal/orders/app/place_order.go
package app

import (
    "context"

    "github.com/vincent-tien/wolf-core/uow"

    "github.com/your-org/my-service/internal/orders/domain"
)

type PlaceOrderCmd struct {
    CustomerID string `validate:"required,uuid4"`
    Total      int64  `validate:"required,gt=0"`
}

type PlaceOrderHandler struct {
    repo domain.OrderRepository
    uow  *uow.UnitOfWork
}

func NewPlaceOrderHandler(repo domain.OrderRepository, uow *uow.UnitOfWork) *PlaceOrderHandler {
    return &PlaceOrderHandler{repo: repo, uow: uow}
}

func (h *PlaceOrderHandler) Handle(ctx context.Context, cmd PlaceOrderCmd) (string, error) {
    order := domain.NewOrder(cmd.CustomerID, cmd.Total)

    err := h.uow.Execute(ctx, order, func(txCtx context.Context) error {
        return h.repo.Save(txCtx, order)
    })
    if err != nil {
        return "", err
    }

    return order.ID(), nil
}
```

---

## Step 6: Create the Module

```go
// internal/orders/module.go
package orders

import (
    "github.com/gin-gonic/gin"

    "github.com/vincent-tien/wolf-core/cqrs"
    "github.com/vincent-tien/wolf-core/event"
    "github.com/vincent-tien/wolf-core/infra/modular"
    "github.com/vincent-tien/wolf-core/runtime"
    "github.com/vincent-tien/wolf-core/uow"
    "github.com/vincent-tien/wolf-core/validator"

    "github.com/your-org/my-service/internal/orders/app"
    "github.com/your-org/my-service/internal/orders/domain"
    "github.com/your-org/my-service/internal/orders/infra"
)

type Module struct {
    placeOrder cqrs.CommandHandler[app.PlaceOrderCmd, string]
}

// Entry returns a CatalogEntry for use in the application manifest.
func Entry() modular.CatalogEntry {
    return modular.CatalogEntry{
        Name:    "orders",
        Factory: newModule,
    }
}

func newModule(c *modular.Container) runtime.Module {
    repo := infra.NewPostgresOrderRepo(c.WriteDB)
    unitOfWork := uow.New(c.TxRunner, c.OutboxStore, "orders")

    handler := cqrs.ChainCommand[app.PlaceOrderCmd, string](
        app.NewPlaceOrderHandler(repo, unitOfWork),
        cqrs.WithCommandValidation[app.PlaceOrderCmd, string](validator.Validate),
        cqrs.WithCommandLogging[app.PlaceOrderCmd, string](c.Logger),
    )

    return &Module{placeOrder: handler}
}

func (m *Module) Name() string { return "orders" }

func (m *Module) RegisterEvents(r *event.TypeRegistry) {
    r.Register("order.placed", domain.OrderPlaced{})
}

func (m *Module) RegisterHTTP(router interface{}) {
    rg := router.(*gin.RouterGroup)
    rg.POST("/orders", m.handlePlaceOrder)
}

func (m *Module) RegisterGRPC(server interface{})               {}
func (m *Module) RegisterSubscribers(sub event.Subscriber) error { return nil }
func (m *Module) OnStart(ctx context.Context) error              { return nil }
func (m *Module) OnStop(ctx context.Context) error               { return nil }
```

---

## Step 7: Wire the Application Entry Point

```go
// cmd/server/main.go
package main

import (
    "context"
    "log"
    "os/signal"
    "syscall"

    "github.com/vincent-tien/wolf-core/infra/bootstrap"
    "github.com/vincent-tien/wolf-core/infra/modular"

    "github.com/your-org/my-service/internal/orders"
)

func main() {
    app, err := bootstrap.New("config.yaml")
    if err != nil {
        log.Fatal(err)
    }

    err = app.RegisterModules([]modular.CatalogEntry{
        orders.Entry(),
        // Add more modules here as your service grows
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

## Step 8: Run

```bash
# Apply migrations
psql -f migrations/001_create_outbox.up.sql

# Start the service
go run ./cmd/server

# Test it
curl -X POST http://localhost:8080/api/v1/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id": "550e8400-e29b-41d4-a716-446655440000", "total": 4999}'
```

---

## Recommended Project Layout

```
my-service/
├── cmd/
│   └── server/
│       └── main.go                 # Entry point — bootstrap + manifest
├── config.yaml                     # Application config
├── migrations/                     # SQL migration files
├── internal/
│   ├── orders/                     # Bounded context: Orders
│   │   ├── domain/
│   │   │   ├── order.go            # Aggregate root
│   │   │   ├── events.go           # Event payload structs
│   │   │   └── repository.go       # Repository interface (port)
│   │   ├── app/
│   │   │   └── place_order.go      # Command handler (use case)
│   │   ├── infra/
│   │   │   └── postgres_repo.go    # Repository implementation (adapter)
│   │   └── module.go               # Module wiring + Entry()
│   └── payments/                   # Another bounded context
│       ├── domain/
│       ├── app/
│       ├── infra/
│       └── module.go
└── go.mod
```

> **Key principle:** Each bounded context follows `domain/ → app/ → infra/` internally. Domain never imports from app or infra. App imports domain only. Infra implements domain interfaces.

---

## What's Next?

- [Architecture Guide](./architecture.md) — understand the patterns in depth
- [Developer Guide](./developer-guide.md) — common tasks, testing, adding features
- [Conventions](./conventions.md) — naming, error handling, concurrency rules
