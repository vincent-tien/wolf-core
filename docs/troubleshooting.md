# Troubleshooting & FAQ

Common issues, error messages, and their solutions.

---

## Build & Compilation

### `go: module requires Go >= 1.26`

wolf-core requires Go 1.26+ for generics features.

```bash
go version          # check your version
go install golang.org/dl/go1.26@latest  # install if needed
```

### `cannot use X as type Y` in generic handler

Generic type parameters must match exactly. Common mismatch:

```go
// ERROR: CreateOrderCmd does not implement cqrs.Command
// This happens when you forget the marker interface is satisfied automatically.
// Command and Query are empty interfaces — any struct satisfies them.
// The real issue is usually a type parameter mismatch:

// WRONG — mismatched result type
cqrs.ChainCommand[CreateOrderCmd, string](handler, ...)
// where handler returns (int, error) instead of (string, error)

// FIX — match the handler's return type
cqrs.ChainCommand[CreateOrderCmd, int](handler, ...)
```

---

## Bootstrap & Startup

### `bootstrap: load config: ...`

Config file not found or malformed YAML.

- Check that the path passed to `bootstrap.New("config.yaml")` exists
- Validate YAML syntax: `python3 -c "import yaml; yaml.safe_load(open('config.yaml'))"`
- If running without a config file, all values must come from environment variables

### `bootstrap: write db pool: ...`

Database connection failed.

- Verify `DB_WRITE_DSN` or `db.write.dsn` is correct
- Check that PostgreSQL is running and accepting connections
- Test manually: `psql "your-dsn-here"`

### `bootstrap: cache client: ...`

Cache initialization failed.

- For development, use `cache.driver: noop` to skip Redis
- For Redis: verify `REDIS_ADDR` or `redis.addr` points to a running instance

### `bootstrap: messaging stream: ...`

Broker connection failed.

- For development, use `broker.driver: inprocess`
- For NATS: check `nats.url` points to a running NATS server with JetStream enabled
- For RabbitMQ: check `rabbitmq.url` is valid AMQP URL

### `modular: duplicate module "X"`

Two modules registered with the same name.

- Check your manifest in `main.go` for duplicate `CatalogEntry` items
- Each module's `Name()` must return a unique string

### `modular: dependency cycle detected among modules`

Module A depends on B which depends on A (or longer cycle).

- Review `DependsOn()` declarations across your modules
- Cycles indicate a design problem — consider merging modules or using events for decoupling

### `modular: module "X" depends on unregistered module "Y"`

A module declares a dependency that isn't in the manifest.

- Add the missing module to your manifest
- Or remove the dependency declaration if it's no longer needed

---

## Runtime

### Events Not Published to Broker

The outbox pattern introduces a delay between domain event emission and broker delivery. If events aren't appearing:

1. **Check outbox worker logs** — look for `outbox worker` log entries
2. **Check outbox table** — `SELECT * FROM outbox_events WHERE status = 'pending' ORDER BY created_at DESC LIMIT 10;`
3. **Check event type registration** — `RegisterEvents()` must register every payload type in the `TypeRegistry`
4. **Check UoW usage** — events are only drained when `uow.Execute()` wraps the repository call:

```go
// CORRECT — events drained inside transaction
h.uow.Execute(ctx, order, func(txCtx context.Context) error {
    return h.repo.Save(txCtx, order)
})

// WRONG — events drained but not persisted to outbox
h.repo.Save(ctx, order)
order.ClearEvents() // events lost!
```

### `uow: txRunner must not be nil` (panic at startup)

A required dependency was not injected.

- This is a wiring bug — check that `Container.TxRunner` is populated
- If using `modular.Container`, ensure `bootstrap.New()` succeeded

### `[UNAUTHORIZED] token has expired`

JWT access token expired.

- Client must refresh using the refresh token endpoint
- Check `jwt.access_token_ttl` in config — default is typically 15 minutes

### `[FORBIDDEN] requires one of roles: admin`

User's JWT claims don't include the required role.

- Verify the user was assigned the role in the auth system
- Check that the JWT was re-issued after role assignment
- If `session.revoke_on_role_change` is false, the old token still has stale claims

### High Outbox Lag / Unhealthy Readiness Check

The outbox worker can't keep up with event production.

- Check broker connectivity — circuit breaker may be open
- Increase `outbox.batch_size` for higher throughput
- Decrease `outbox.poll_interval` for lower latency
- Enable `outbox.notify_enabled` for LISTEN/NOTIFY wake-up
- Check for failed events: `SELECT * FROM outbox_events WHERE status = 'failed';`

### `too many requests, please slow down`

Rate limiter triggered.

- Per-IP rate limiting is configured via `rate_limit.rps` and `rate_limit.burst`
- Set `rate_limit.rps: 0` to disable (development only)
- Behind a load balancer: configure `http.trusted_proxies` so the real client IP is used

---

## Testing

### Race Condition Detected

```
WARNING: DATA RACE
```

Always run tests with `-race`. Common causes:

- Shared slice/map accessed from multiple goroutines without mutex
- `sync.WaitGroup.Add` called after goroutine launch
- Struct with mutex copied (use pointer receivers)

### Goroutine Leak Detected

```
goleak: found unexpected goroutines
```

A goroutine wasn't properly shut down after the test:

- Ensure all background workers receive cancellation via context
- Close channels and connections in test cleanup
- Use `t.Cleanup()` to register teardown functions

### `context deadline exceeded` in Tests

Test is hitting a real timeout:

- Use shorter timeouts in test config
- Mock external dependencies instead of connecting to real services
- Use `context.WithTimeout(context.Background(), 5*time.Second)` for test contexts

---

## Common Mistakes

| Mistake | Problem | Fix |
|---------|---------|-----|
| Import `infra/` in domain package | Breaks layer dependency rule | Define interface in domain, implement in infra |
| `return err` without wrapping | No context in error chain | `return fmt.Errorf("context: %w", err)` |
| Log AND return error | Duplicate log entries | Do one or the other, not both |
| `panic` in request handler | Crashes the process (if recovery middleware misses it) | Return `errors.NewInternal(err)` |
| Save aggregate outside UoW | Events not persisted to outbox | Always use `uow.Execute()` |
| Forget `RegisterEvents()` | Deserialization fails for event payloads | Register every payload type |
| Use `%v` instead of `%w` | Callers can't unwrap the error | Use `%w` when unwrapping is needed |
| Shared mutable state without mutex | Data race under concurrent load | Use mutex or channels |
| `time.Now()` in domain logic | Non-deterministic, untestable | Inject `clock.Clock` interface |
| Array index as map key in tests | Flaky test due to map iteration order | Use deterministic keys |

---

## Getting Help

- Check the [Architecture Guide](./architecture.md) for pattern explanations
- Check the [Developer Guide](./developer-guide.md) for code examples
- Check the [Glossary](./glossary.md) for terminology
- Read GoDoc comments on the types you're using — they contain usage notes and caveats
