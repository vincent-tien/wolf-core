// Package db provides database connection pool helpers for the wolf-be service.
package db

import (
	"context"
	"database/sql"
)

// Conn is a minimal interface that unifies *sql.DB and *sql.Tx so that
// repository methods can accept either without knowing whether they are
// executing inside a transaction or against the raw pool.
//
// Both *sql.DB and *sql.Tx satisfy this interface from the standard library,
// which is verified by the compile-time assertions below.
type Conn interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

// Compile-time interface satisfaction checks. These produce a clear compiler
// error if the standard library types no longer satisfy Conn.
var (
	_ Conn = (*sql.DB)(nil)
	_ Conn = (*sql.Tx)(nil)
)
