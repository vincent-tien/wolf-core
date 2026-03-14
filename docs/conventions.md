# Conventions & Standards

Coding standards enforced across wolf-core and consuming services.

---

## Naming

### Packages

- Single lowercase word: `event`, `auth`, `tx`, `vo`, `uow`
- Sub-packages for logical grouping: `infra/events/outbox`, `infra/middleware/http`
- No `utils`, `helpers`, `common` — name by what it does

### Files

- Snake_case per Go convention: `unit_of_work.go`, `jwt_service.go`
- Test files: `*_test.go` in the same package
- Benchmark files: `*_bench_test.go`

### Types

| Kind | Convention | Example |
|------|-----------|---------|
| Interfaces | Verb or noun (no `I` prefix) | `Runner`, `Publisher`, `Subscriber` |
| Structs | Noun | `UnitOfWork`, `AppError`, `UserClaims` |
| Constructors | `New` + type name | `NewBase()`, `NewMoney()`, `NewEvent()` |
| Options | `With` prefix | `WithCorrelationID()`, `WithSource()` |
| Predicates | `Is` prefix | `IsNotFound()`, `IsExpired()` |
| Adapters | `As` prefix | `AsVoidCommand()` |

### Event Types

Dot-separated, past tense: `order.placed`, `product.price_changed`, `user.registered`

Format: `<aggregate>.<what_happened>`

### Error Codes

Uppercase with underscores: `NOT_FOUND`, `RATE_LIMITED`, `UNAVAILABLE`

---

## Error Handling

### Always Wrap with Context

```go
// GOOD
return fmt.Errorf("orders: save order %s: %w", order.ID(), err)

// BAD — no context, hard to trace
return err
```

### Handle Once: Log OR Return, Never Both

```go
// GOOD — return to caller
if err != nil {
    return fmt.Errorf("fetch user: %w", err)
}

// GOOD — log at the boundary (handler/middleware level)
if err != nil {
    logger.Error("fetch user failed", zap.Error(err))
    return errors.NewInternal(err)
}

// BAD — logged twice, noisy
if err != nil {
    logger.Error("fetch user failed", zap.Error(err))
    return fmt.Errorf("fetch user: %w", err)
}
```

### Use Typed Errors

```go
// GOOD — typed, machine-readable
return errors.NewNotFound("order", id)
return errors.NewValidation("email", "must be valid email format")
return errors.NewConflict("order with this reference already exists")

// BAD — stringly-typed, no error code
return fmt.Errorf("order not found: %s", id)
```

### Check Errors with Predicates

```go
// GOOD
if errors.IsNotFound(err) {
    // handle 404
}

// BAD — fragile string matching
if strings.Contains(err.Error(), "not found") {
    // handle 404
}
```

### Intentional `%w` vs `%v`

- Use `%w` when callers should be able to unwrap and inspect the cause
- Use `%v` when you want to hide implementation details from callers

---

## Concurrency

### Every Goroutine Has a Stop Mechanism

```go
// GOOD — context-based cancellation
func (w *Worker) Start(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return nil
        case msg := <-w.inbox:
            w.process(msg)
        }
    }
}
```

### No Shared Mutable State Without Synchronization

```go
// GOOD — mutex-protected
type SafeCounter struct {
    mu    sync.Mutex
    count int
}

func (c *SafeCounter) Inc() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}

// BAD — data race
type Counter struct {
    count int // accessed from multiple goroutines
}
```

### WaitGroup.Add Before Goroutine Launch

```go
// GOOD
var wg sync.WaitGroup
wg.Add(1)
go func() {
    defer wg.Done()
    // work
}()

// BAD — race between Add and goroutine start
go func() {
    wg.Add(1) // might execute after wg.Wait()
    defer wg.Done()
}()
```

### Background Tasks Use context.Background()

```go
// GOOD — independent of request lifecycle
go func() {
    ctx := context.Background()
    w.processAsync(ctx, msg)
}()

// BAD — request context may cancel mid-processing
go func() {
    w.processAsync(reqCtx, msg) // cancelled when HTTP response sent
}()
```

---

## Constructor Validation

Constructors panic on nil dependencies. This is a deliberate design choice — invalid wiring is a programmer error caught at startup, not a runtime condition.

```go
// This is correct for wolf-core
func New(txRunner tx.Runner, outboxStore OutboxInserter, source string) *UnitOfWork {
    if txRunner == nil {
        panic("uow: txRunner must not be nil")
    }
    // ...
}
```

> **When to panic:** Only in constructors for required dependencies that indicate a wiring bug. Never panic in request handlers or business logic — return errors instead.

---

## Value Object Rules

1. **Immutable** — no setters, operations return new instances
2. **Validated at construction** — `NewMoney()` rejects invalid currency
3. **Compared by value** — `Equals()` method, not pointer identity
4. **Contains behavior** — `Add()`, `Subtract()`, not just data holders

```go
// GOOD — immutable, validated
total, err := vo.NewMoney(4999, "USD")
discounted := total.Subtract(vo.NewMoney(500, "USD"))

// BAD — mutable, unvalidated
total := Money{Amount: 4999, Currency: "whatever"}
total.Amount -= 500
```

---

## Aggregate Rules

1. **One aggregate = one transaction boundary** — never save two aggregates in one transaction (use `ExecuteMulti` only when semantically required)
2. **Reference other aggregates by ID** — never hold object references
3. **State transitions emit events** — call `AddDomainEvent()` inside business methods
4. **Increment version** on every state change — for optimistic concurrency
5. **SetVersion/SetCreatedAt** are for rehydration only — never in use cases

---

## Testing Standards

### Test Naming

```go
func TestOrder_Place_EmitsEvent(t *testing.T) { ... }
func TestMoney_Add_DifferentCurrency_ReturnsError(t *testing.T) { ... }
```

Format: `Test<Type>_<Method>_<Scenario>_<Expected>`

### Table-Driven Tests

```go
func TestMoney_NewMoney(t *testing.T) {
    tests := []struct {
        name     string
        amount   int64
        currency string
        wantErr  bool
    }{
        {"valid", 100, "USD", false},
        {"zero amount", 0, "USD", false},
        {"negative", -1, "USD", true},
        {"invalid currency", 100, "XX", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := vo.NewMoney(tt.amount, tt.currency)
            if tt.wantErr {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### What to Test

| Layer | What to Test | How |
|-------|-------------|-----|
| Aggregate | State transitions, invariants, event emission | Unit test, no mocks |
| Value Object | Validation, arithmetic, equality | Unit test, table-driven |
| Command Handler | Orchestration, error paths | Unit test with mock repo/tx |
| Query Handler | SQL correctness, error mapping | Integration test with real DB |
| Middleware | Cross-cutting behavior | Unit test with mock handler |

### Goroutine Leak Detection

Use `goleak.VerifyTestMain` in packages that spawn goroutines:

```go
func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}
```

---

## Commit Format

Conventional commits:

```
feat(orders): add place order command handler
fix(outbox): prevent duplicate event publishing on retry
refactor(auth): extract role checking into UserClaims method
test(aggregate): add table-driven tests for ClearEvents
docs(readme): add quick start example
chore(deps): bump pgx to v5.8.0
```

Format: `<type>(<scope>): <description>`

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`, `ci`
