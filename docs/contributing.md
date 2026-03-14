# Contributing

Guidelines for contributing to wolf-core.

---

## Before You Start

1. **Check existing packages** — wolf-core may already solve your problem
2. **Open an issue first** for significant changes — discuss before implementing
3. **Read [Conventions](./conventions.md)** — naming, error handling, testing standards

---

## Development Setup

```bash
git clone https://github.com/vincent-tien/wolf-core.git
cd wolf-core
go build ./...
go test ./... -race -count=1 -shuffle=on
```

No external tooling required beyond the Go toolchain. No Makefile, Docker, or database needed for core package development.

---

## Branch Naming

```
feat/add-saga-pattern
fix/outbox-duplicate-events
refactor/extract-cache-middleware
docs/add-messenger-guide
```

Format: `<type>/<short-description>`

---

## Pull Request Process

### 1. Create a Branch

```bash
git checkout -b feat/your-feature main
```

### 2. Implement

- Follow the [layer dependency rule](./architecture.md) — domain packages must not import `infra/`
- Write tests alongside code
- Keep files under 200 lines — split when exceeded

### 3. Validate

```bash
go build ./...                              # must compile
go vet ./...                                # must pass
go test ./... -race -count=1 -shuffle=on    # must pass, no races
```

### 4. Commit

Use [conventional commits](./conventions.md#commit-format):

```bash
git commit -m "feat(cqrs): add retry middleware for command handlers"
```

### 5. Open PR

- **Title:** conventional commit format
- **Description:** what changed, why, and how to test
- **Link related issue** if one exists

### 6. Review Checklist

Reviewers check:

- [ ] Compiles cleanly (`go build ./...`)
- [ ] Tests pass with race detector (`go test -race`)
- [ ] New public types have GoDoc comments
- [ ] No `infra/` imports in domain/application packages
- [ ] Error handling follows wrap-with-context pattern
- [ ] No `panic` outside of constructor nil-checks
- [ ] Goroutines have stop mechanisms
- [ ] Value objects are immutable

---

## Quality Gates

### Must Pass (blocking)

| Gate | Command | Threshold |
|------|---------|-----------|
| Compile | `go build ./...` | Zero errors |
| Static analysis | `go vet ./...` | Zero warnings |
| Tests | `go test ./... -race -count=1 -shuffle=on` | 100% pass |
| Coverage (changed code) | `go test -coverprofile` | 80% minimum |

### Should Pass (advisory)

| Gate | Check |
|------|-------|
| File size | Code files under 200 lines |
| Function size | Functions under 40 lines |
| GoDoc | All exported types and functions documented |

---

## What Belongs in wolf-core

wolf-core is a **shared kernel** — it should contain only patterns that are reused across multiple bounded contexts.

### Good Candidates

- Domain modeling primitives (new value objects, specification helpers)
- CQRS middleware (retry, circuit breaker, rate limiting)
- Messaging abstractions (new broker adapters, stream utilities)
- Infrastructure patterns (new cache backends, observability helpers)

### Does NOT Belong

- Business-domain-specific logic (order processing, payment rules)
- Application-specific configuration
- UI/frontend concerns
- One-off utilities for a single service

### Decision Criteria

Ask yourself:

1. **Would 2+ services benefit from this?** If no → keep it in your service
2. **Is it a pattern or a feature?** Patterns belong here; features belong in services
3. **Does it introduce new dependencies?** Minimize — each new dep affects all consumers

---

## Adding a New Package

### Domain/Application Layer

1. Create directory at project root (e.g., `saga/`)
2. Write the package with zero `infra/` imports
3. Add comprehensive tests
4. Document exported types with GoDoc

### Infrastructure Layer

1. Create under `infra/` (e.g., `infra/events/kafka/`)
2. Implement interfaces from domain/application layer
3. Wire into bootstrap if it's a platform dependency
4. Add config struct to `infra/config/config.go` if needed
5. Add tests (integration tests may require external services)

### Documentation

1. Update [Code Structure](./code-structure.md) with the new package
2. Update [README.md](../README.md) package table if it's a top-level package
3. Add GoDoc comments to all exported types and functions

---

## Versioning

wolf-core follows [Semantic Versioning](https://semver.org/):

- **MAJOR** — breaking changes to public interfaces
- **MINOR** — new packages, types, or methods (backward-compatible)
- **PATCH** — bug fixes, performance improvements

### What Counts as Breaking

- Removing or renaming an exported type, function, or method
- Changing a function signature (parameters or return types)
- Changing interface contracts (adding methods to an interface)
- Removing a package

### What's NOT Breaking

- Adding new packages
- Adding new exported functions or methods
- Adding new fields to structs (unless the struct is used in comparisons)
- Adding new constants or error codes
- Internal refactoring that doesn't change the public API
