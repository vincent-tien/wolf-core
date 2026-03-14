// Package db provides database connection pool helpers for the wolf-be service.
package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/vincent-tien/wolf-core/tx"
)

// TxManager manages database transactions. Implementations must be safe for
// concurrent use.
type TxManager interface {
	// WithTx executes fn inside a database transaction. The transaction is
	// committed when fn returns nil and rolled back on any error or panic.
	WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error
}

// WithTxResult is a generic helper that executes fn inside a transaction and
// returns both the result value and any error. It wraps TxManager semantics
// without requiring the caller to own a TxManager instance.
//
// The zero value of T is returned on any error.
func WithTxResult[T any](ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) (T, error)) (T, error) {
	var zero T

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return zero, fmt.Errorf("db: begin tx: %w", err)
	}

	// Ensure rollback on panic so callers do not need to manage this themselves.
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // re-panic so the caller's panic handler sees the original value
		}
	}()

	result, err := fn(tx)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return zero, fmt.Errorf("db: fn failed (%w); rollback also failed: %v", err, rbErr)
		}

		return zero, err
	}

	if err := tx.Commit(); err != nil {
		return zero, fmt.Errorf("db: commit tx: %w", err)
	}

	return result, nil
}

// pgTxRunner adapts TxManager into the domain-friendly tx.Runner interface.
// It begins a transaction via TxManager, injects the *sql.Tx into the context
// via tx.Inject, and passes the enriched context to fn.
type pgTxRunner struct {
	mgr TxManager
}

// NewTxRunner returns a tx.Runner backed by the given TxManager.
func NewTxRunner(mgr TxManager) tx.Runner {
	return &pgTxRunner{mgr: mgr}
}

// RunInTx implements tx.Runner.
func (r *pgTxRunner) RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	return r.mgr.WithTx(ctx, func(sqlTx *sql.Tx) error {
		txCtx := tx.Inject(ctx, sqlTx)
		return fn(txCtx)
	})
}

// pgTxManager is a PostgreSQL-backed implementation of TxManager.
type pgTxManager struct {
	db *sql.DB
}

// NewTxManager returns a TxManager backed by the provided *sql.DB.
func NewTxManager(db *sql.DB) TxManager {
	return &pgTxManager{db: db}
}

// WithTx implements TxManager. It begins a transaction, calls fn, and commits
// on success. Any panic inside fn triggers an immediate rollback before the
// panic is re-propagated upward. On fn error the transaction is rolled back.
func (m *pgTxManager) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // re-panic with the original value
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("db: fn failed (%w); rollback also failed: %v", err, rbErr)
		}

		return fmt.Errorf("db: tx fn: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit tx: %w", err)
	}

	return nil
}
