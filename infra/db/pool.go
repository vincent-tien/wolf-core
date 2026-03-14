// Package db provides database connection pool helpers for the wolf-be service.
// It supports read/write splitting via separate pools for the primary and
// replica PostgreSQL instances.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	// Register the pgx/v5 driver with database/sql. We use pgx instead of lib/pq
	// because pgx speaks PostgreSQL's binary wire format (~15% latency reduction),
	// provides native LISTEN/NOTIFY support (used by outbox Notifier), and exposes
	// structured error types (pgconn.PgError) instead of string-based error codes.
	//
	// This is a stdlib drop-in: all code continues to use *sql.DB, *sql.Tx, etc.
	// The driver name registered is "pgx" (used in sql.Open below).
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/vincent-tien/wolf-core/infra/config"
)

const pingTimeout = 5 * time.Second

// buildDSN appends pgx and PostgreSQL runtime parameters to the base DSN.
// This ensures statement_timeout, idle_in_transaction_session_timeout, and
// simple protocol mode are applied on every connection without requiring
// operators to manually edit DSN strings.
func buildDSN(cfg config.PoolConfig) (string, error) {
	u, err := url.Parse(cfg.DSN)
	if err != nil {
		return "", fmt.Errorf("db: parse dsn: %w", err)
	}

	q := u.Query()
	if cfg.StatementTimeout > 0 {
		q.Set("statement_timeout", fmt.Sprintf("%d", cfg.StatementTimeout))
	}
	if cfg.IdleInTransactionTimeout > 0 {
		q.Set("idle_in_transaction_session_timeout", fmt.Sprintf("%d", cfg.IdleInTransactionTimeout))
	}
	if cfg.SimpleProtocol {
		q.Set("default_query_exec_mode", "simple_protocol")
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// NewPool opens a new *sql.DB connection pool using the provided PoolConfig,
// applies all pool tuning parameters, and verifies connectivity with a ping.
// The caller is responsible for closing the returned pool.
func NewPool(cfg config.PoolConfig) (*sql.DB, error) {
	dsn, err := buildDSN(cfg)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open pool: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		// Close the pool before returning to avoid a resource leak.
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("db: ping failed (%w); also failed to close pool: %v", err, closeErr)
		}

		return nil, fmt.Errorf("db: ping pool: %w", err)
	}

	return db, nil
}

// NewWritePool opens and verifies a connection pool to the primary (write)
// database using DBConfig.Write pool settings.
func NewWritePool(cfg config.DBConfig) (*sql.DB, error) {
	pool, err := NewPool(cfg.Write)
	if err != nil {
		return nil, fmt.Errorf("db: new write pool: %w", err)
	}

	return pool, nil
}

// NewReadPool opens and verifies a connection pool to the replica (read)
// database using DBConfig.Read pool settings.
func NewReadPool(cfg config.DBConfig) (*sql.DB, error) {
	pool, err := NewPool(cfg.Read)
	if err != nil {
		return nil, fmt.Errorf("db: new read pool: %w", err)
	}

	return pool, nil
}
