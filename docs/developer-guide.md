# Developer Guide

Practical workflows for developing with and contributing to wolf-core.

---

## Build & Test

```bash
# Compile all packages
go build ./...

# Run full test suite with race detector
go test ./... -race -count=1 -shuffle=on

# Run a single test
go test ./uow -run TestUnitOfWork_Execute

# Run tests in a specific package
go test ./event/...

# Run benchmarks
go test ./... -bench=. -benchmem

# Generate coverage report
go test ./... -coverprofile=cover.out
go tool cover -html=cover.out
```

> **Always run with `-race`** in development. Most production Go bugs are concurrency issues that `-race` catches at test time.

---

## Adding a New Feature to wolf-core

### Checklist

- [ ] Identify which layer the feature belongs to (domain / application / infra)
- [ ] Check if the feature can extend an existing package vs creating a new one
- [ ] Write the interface/contract first, implementation second
- [ ] Keep domain packages free of infrastructure imports
- [ ] Write tests alongside the code (not after)
- [ ] Run `go vet ./...` and `go test ./... -race` before committing

### Adding a New Domain Package

1. Create the package directory under the project root (e.g., `saga/`)
2. Define interfaces and types — pure Go, no external imports
3. Write unit tests
4. Export only what consumers need

```go
// saga/saga.go
package saga

import "context"

type Step[T any] interface {
    Execute(ctx context.Context, state T) (T, error)
    Compensate(ctx context.Context, state T) error
}
```

### Adding a New Infrastructure Adapter

1. Create under `infra/` (e.g., `infra/events/kafka/`)
2. Implement the interface from the domain/application layer
3. Wire into `infra/bootstrap` if it's a platform concern
4. Add configuration to `infra/config/config.go` if needed

### Adding a New Module (in a consuming service)

Follow the golden path in [Getting Started](./getting-started.md). The module checklist:

- [ ] `domain/` — aggregate, events, repository interface
- [ ] `app/` — command/query handlers
- [ ] `infra/` — repository implementation
- [ ] `module.go` — implements `runtime.Module`, exports `Entry()`
- [ ] Register event types in `RegisterEvents()`
- [ ] Mount routes in `RegisterHTTP()` and/or `RegisterGRPC()`
- [ ] Add to manifest in `cmd/server/main.go`

---

## Common Development Tasks

### Creating a Command Handler

```go
// 1. Define the command struct with validation tags
type CreateProductCmd struct {
    Name  string `validate:"required,min=1,max=255"`
    Price int64  `validate:"required,gt=0"`
}

// 2. Implement the handler
type CreateProductHandler struct {
    repo ProductRepository
    uow  *uow.UnitOfWork
}

func (h *CreateProductHandler) Handle(ctx context.Context, cmd CreateProductCmd) (string, error) {
    product := NewProduct(cmd.Name, cmd.Price)
    err := h.uow.Execute(ctx, product, func(txCtx context.Context) error {
        return h.repo.Save(txCtx, product)
    })
    return product.ID(), err
}

// 3. Wire with middleware in module.go
handler := cqrs.ChainCommand[CreateProductCmd, string](
    &CreateProductHandler{repo: repo, uow: unitOfWork},
    cqrs.WithCommandValidation[CreateProductCmd, string](validator.Validate),
    cqrs.WithCommandLogging[CreateProductCmd, string](logger),
    cqrs.WithCommandMetrics[CreateProductCmd, string](metrics),
)
```

### Creating a Query Handler

```go
type GetProductQuery struct {
    ID string `validate:"required,uuid4"`
}

type GetProductHandler struct {
    readDB *sql.DB
}

func (h *GetProductHandler) Handle(ctx context.Context, q GetProductQuery) (*ProductDTO, error) {
    // Query directly from read DB — no aggregate, no UoW needed
    row := h.readDB.QueryRowContext(ctx, "SELECT id, name, price FROM products WHERE id = $1", q.ID)
    var dto ProductDTO
    if err := row.Scan(&dto.ID, &dto.Name, &dto.Price); err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, apperrors.NewNotFound("product", q.ID)
        }
        return nil, apperrors.NewInternal(err)
    }
    return &dto, nil
}
```

### Adding Domain Events

```go
// 1. Define the payload struct
type ProductPriceChanged struct {
    ProductID string `json:"product_id"`
    OldPrice  int64  `json:"old_price"`
    NewPrice  int64  `json:"new_price"`
}

// 2. Register in module's RegisterEvents
func (m *Module) RegisterEvents(r *event.TypeRegistry) {
    r.Register("product.price_changed", ProductPriceChanged{})
}

// 3. Emit from aggregate method
func (p *Product) ChangePrice(newPrice int64) {
    old := p.price
    p.price = newPrice
    p.SetUpdatedAt(time.Now().UTC())
    p.IncrementVersion()
    p.AddDomainEvent("product.price_changed", ProductPriceChanged{
        ProductID: p.ID(),
        OldPrice:  old,
        NewPrice:  newPrice,
    })
}
```

### Subscribing to Events

```go
// In-process (same service)
func (m *Module) RegisterSubscribers(sub event.Subscriber) error {
    return sub.Subscribe("order.placed", event.HandlerFunc(func(ctx context.Context, evt event.Event) error {
        payload := evt.Payload().(OrderPlaced)
        // React to the event
        return nil
    }))
}

// Cross-service (via messaging stream)
func (m *Module) RegisterStreams(stream messaging.Stream) error {
    return stream.Subscribe(ctx, "order.placed", func(msg messaging.Message) error {
        // Process message from broker
        return msg.Ack()
    })
}
```

### Using the Cache

```go
// Cache-aside via CQRS middleware
handler := cqrs.ChainQuery[GetProductQuery, *ProductDTO](
    &GetProductHandler{readDB: readDB},
    cqrs.WithQueryCaching[GetProductQuery, *ProductDTO](
        cache,
        func(q GetProductQuery) string { return "product:" + q.ID },
        5*time.Minute,
    ),
)

// Invalidate after mutation
cmdHandler := cqrs.ChainCommand[UpdateProductCmd, cqrs.Void](
    &UpdateProductHandler{repo: repo, uow: unitOfWork},
    cqrs.WithCommandCacheInvalidation[UpdateProductCmd, cqrs.Void](
        cache,
        func(cmd UpdateProductCmd) []string { return []string{"product:" + cmd.ID} },
    ),
)
```

---

## Testing Patterns

### Unit Testing Aggregates

```go
func TestOrder_Place(t *testing.T) {
    order := NewOrder("cust-1", 4999)

    assert.Equal(t, "pending", order.Status())
    assert.True(t, order.HasEvents())

    events := order.ClearEvents()
    require.Len(t, events, 1)
    assert.Equal(t, "order.placed", events[0].EventType())
}
```

### Unit Testing Command Handlers

```go
func TestPlaceOrder_Handle(t *testing.T) {
    repo := &mockOrderRepo{}
    txRunner := &mockTxRunner{} // runs fn immediately, no real DB
    unitOfWork := uow.New(txRunner, &mockOutbox{}, "test")

    handler := &PlaceOrderHandler{repo: repo, uow: unitOfWork}

    id, err := handler.Handle(context.Background(), PlaceOrderCmd{
        CustomerID: "cust-1",
        Total:      4999,
    })

    require.NoError(t, err)
    assert.NotEmpty(t, id)
    assert.True(t, repo.saveCalled)
}
```

### Testing with FakeClock

```go
func TestToken_Expiry(t *testing.T) {
    fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
    clk := clock.FakeClock{Fixed: fixed}

    token := issueToken(clk)
    assert.False(t, token.IsExpired(clk.Now()))

    // Advance time past expiry
    clk.Fixed = fixed.Add(2 * time.Hour)
    assert.True(t, token.IsExpired(clk.Now()))
}
```

### Goroutine Leak Detection

wolf-core uses `go.uber.org/goleak` to detect leaked goroutines:

```go
func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}
```

---

## Debugging Tips

### Transaction Context

The `tx.Runner` injects the transaction into context. If your repository isn't seeing the transaction, check:

1. You're using `txCtx` (from the callback), not the outer `ctx`
2. Your repository calls the correct context-aware DB method

### Event Not Published

If domain events aren't reaching the broker:

1. Check that `RegisterEvents()` registers the payload type in `TypeRegistry`
2. Verify the outbox worker is running (check logs for `outbox worker` messages)
3. Confirm the aggregate's `AddDomainEvent()` was called inside a state transition
4. Ensure `UnitOfWork.Execute()` wraps the repo save (not called separately)

### Module Not Loading

If a module isn't responding to requests:

1. Check `config.yaml` — modules not in the `modules` map are enabled by default, but explicitly set `modules.your_module: false` disables it
2. Verify `Entry()` is in the manifest slice in `main.go`
3. Check logs for `module registered` or dependency cycle errors
