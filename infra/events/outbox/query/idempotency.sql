-- name: GetIdempotencyKey :one
SELECT key, response_code, response_body, created_at, expires_at
FROM idempotency_keys
WHERE key = $1 AND expires_at > NOW();

-- name: SetIdempotencyKey :exec
INSERT INTO idempotency_keys (key, response_code, response_body, created_at, expires_at)
VALUES ($1, $2, $3, NOW(), $4)
ON CONFLICT (key) DO UPDATE SET response_code = $2, response_body = $3, expires_at = $4;

-- name: CleanupExpiredKeys :execrows
DELETE FROM idempotency_keys WHERE expires_at < NOW();
