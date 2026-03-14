// Package db provides database connection pool helpers for the wolf-be service.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// HealthChecker checks the connectivity of the write and read database pools.
// Use NewHealthChecker to construct a valid instance.
type HealthChecker struct {
	writeDB *sql.DB
	readDB  *sql.DB
}

// NewHealthChecker returns a HealthChecker that checks both writeDB and readDB.
func NewHealthChecker(writeDB, readDB *sql.DB) *HealthChecker {
	return &HealthChecker{
		writeDB: writeDB,
		readDB:  readDB,
	}
}

// CheckWrite verifies that the primary (write) database connection is alive by
// issuing a PingContext. It propagates the context so callers can enforce a
// deadline.
func (h *HealthChecker) CheckWrite(ctx context.Context) error {
	if err := h.writeDB.PingContext(ctx); err != nil {
		return fmt.Errorf("db: write pool health check: %w", err)
	}

	return nil
}

// CheckRead verifies that the replica (read) database connection is alive by
// issuing a PingContext. It propagates the context so callers can enforce a
// deadline.
func (h *HealthChecker) CheckRead(ctx context.Context) error {
	if err := h.readDB.PingContext(ctx); err != nil {
		return fmt.Errorf("db: read pool health check: %w", err)
	}

	return nil
}

// CheckAll runs both CheckWrite and CheckRead and returns a combined error if
// either pool is unhealthy. Both checks are always attempted so the caller
// receives a complete picture of the database health in a single call.
func (h *HealthChecker) CheckAll(ctx context.Context) error {
	writeErr := h.CheckWrite(ctx)
	readErr := h.CheckRead(ctx)

	return errors.Join(writeErr, readErr)
}
