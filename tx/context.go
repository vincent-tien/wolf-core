// Package tx provides context-carried transaction propagation. It decouples
// domain and application layers from database/sql by storing the raw
// transaction as any in context.Context.
//
// Why `any` instead of `*sql.Tx`: the shared kernel must not import
// database/sql (strict dependency direction rule). Repository adapters in
// the infrastructure layer type-assert the raw value back to *sql.Tx.
// This keeps the domain and application layers database-agnostic.
//
// Usage flow:
//  1. db.TxRunner.RunInTx() starts a *sql.Tx and calls tx.Inject(ctx, sqlTx).
//  2. Use case / UoW passes the enriched ctx to repository methods.
//  3. Repository calls tx.Extract(ctx) to get the raw value, asserts *sql.Tx.
//  4. If no tx in ctx, repository falls back to its default DB pool.
package tx

import "context"

type ctxKey struct{}

// Inject stores a raw transaction value in the context. The value is typically
// a *sql.Tx but the tx package intentionally avoids that import.
func Inject(ctx context.Context, rawTx any) context.Context {
	return context.WithValue(ctx, ctxKey{}, rawTx)
}

// Extract retrieves the raw transaction from ctx. Returns (nil, false) when
// no transaction has been injected.
func Extract(ctx context.Context) (any, bool) {
	v := ctx.Value(ctxKey{})
	return v, v != nil
}
