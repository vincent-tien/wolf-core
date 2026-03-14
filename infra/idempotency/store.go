// Package idempotency provides HTTP-level idempotency key deduplication backed
// by a PostgreSQL table. Clients supply an Idempotency-Key header; the first
// response for a key is cached in the database and replayed on subsequent
// requests with the same key, preventing duplicate side-effects.
package idempotency

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CachedResponse holds the serialised HTTP response associated with an
// idempotency key.
type CachedResponse struct {
	// StatusCode is the HTTP status code of the original response.
	StatusCode int
	// Body is the raw JSON response body.
	Body []byte
	// CreatedAt is the time the response was first cached.
	CreatedAt time.Time
	// ExpiresAt is the time after which the cached entry may be evicted.
	ExpiresAt time.Time
}

// Store persists and retrieves idempotency key responses from the database.
type Store struct {
	db *sql.DB
}

// NewStore creates a *Store backed by the provided connection pool.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Get retrieves the cached response for key. It returns nil, nil when no entry
// exists for the key (i.e. first-time request).
func (s *Store) Get(ctx context.Context, key string) (*CachedResponse, error) {
	const query = `
		SELECT response_code, response_body, created_at, expires_at
		FROM   idempotency_keys
		WHERE  key = $1
		  AND  expires_at > NOW()`

	var resp CachedResponse
	err := s.db.QueryRowContext(ctx, query, key).Scan(
		&resp.StatusCode,
		&resp.Body,
		&resp.CreatedAt,
		&resp.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("idempotency: get key %q: %w", key, err)
	}

	return &resp, nil
}

// Set persists a CachedResponse for key. If a row with the same key already
// exists it is replaced (upsert semantics).
func (s *Store) Set(ctx context.Context, key string, resp CachedResponse) error {
	const query = `
		INSERT INTO idempotency_keys (key, response_code, response_body, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (key) DO UPDATE
		  SET response_code = EXCLUDED.response_code,
		      response_body = EXCLUDED.response_body,
		      created_at    = EXCLUDED.created_at,
		      expires_at    = EXCLUDED.expires_at`

	if _, err := s.db.ExecContext(ctx, query,
		key,
		resp.StatusCode,
		resp.Body,
		resp.CreatedAt,
		resp.ExpiresAt,
	); err != nil {
		return fmt.Errorf("idempotency: set key %q: %w", key, err)
	}

	return nil
}

// Cleanup removes all expired idempotency key entries from the database.
// It should be called periodically (e.g. from a background worker) to prevent
// unbounded table growth.
func (s *Store) Cleanup(ctx context.Context) error {
	const query = `DELETE FROM idempotency_keys WHERE expires_at <= NOW()`

	if _, err := s.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("idempotency: cleanup: %w", err)
	}

	return nil
}
