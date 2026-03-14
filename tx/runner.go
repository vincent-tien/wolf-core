// runner.go — Transaction runner port (interface) for the shared kernel.
//
// This interface lives in shared/tx (not platform/db) so that use cases and
// UoW can depend on it without importing infrastructure packages.
// The concrete implementation (db.TxRunner) lives in platform/db.
package tx

import "context"

// Runner executes a function inside a database transaction. The transaction
// is injected into the context passed to fn via Inject, so callees can
// retrieve it with Extract without importing database/sql.
//
// Error semantics: if fn returns non-nil error, the transaction is rolled back.
// If fn returns nil, the transaction is committed. The Runner implementation
// handles Begin/Commit/Rollback — callers never touch *sql.Tx directly.
type Runner interface {
	RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error
}
