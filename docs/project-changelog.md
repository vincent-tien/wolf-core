# Changelog

All notable changes to `wolf-core` are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/).

## [v0.3.0] — 2026-03-20

### Added
- `grpc.enabled` config field (`*bool`, nil defaults to true for backward compat)
- `GRPCConfig.IsEnabled()` helper method
- `grpc.NewNoop()` — non-listening server; `RegisterGRPC` still works, `Start`/`Stop` are safe no-ops
- `httputil` package with customizable `ErrorMapper`

### Changed
- Bootstrap conditionally skips gRPC listener when `grpc.enabled: false` (logs warning)
- Removed `MaxActive` session limit from config

### Fixed
- Cast `[]byte` to `string` for JSONB columns in outbox store

### Docs
- Document `grpc.enabled` in getting-started, architecture, code-structure

## [v0.2.0] — 2026-03-15

### Added
- CQRS tracing, inbox dedup metric, and resilient token blacklist
- `vo.EncodeCursor` / `vo.DecodeCursor` for keyset pagination

### Changed
- Standardize HTTP response helpers, eliminate raw gin abort calls

## [v0.1.0] — 2026-03-14

### Added
- Initial wolf-core platform library (extracted from wolf-be)
- Comprehensive project documentation
