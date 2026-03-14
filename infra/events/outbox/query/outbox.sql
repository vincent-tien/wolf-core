-- name: InsertOutboxEvent :exec
INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, payload, trace_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetUnpublishedEvents :many
SELECT id, aggregate_type, aggregate_id, event_type, payload, trace_id, created_at, published_at, retry_count, last_error
FROM outbox_events
WHERE published_at IS NULL
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkEventsPublished :exec
UPDATE outbox_events SET published_at = NOW() WHERE id = ANY($1::uuid[]);

-- name: IncrementRetryCount :exec
UPDATE outbox_events SET retry_count = retry_count + 1, last_error = $2 WHERE id = $1;

-- name: CleanupPublishedEvents :execrows
DELETE FROM outbox_events WHERE published_at IS NOT NULL AND published_at < NOW() - $1::interval;

-- name: GetOutboxQueueDepth :one
SELECT COUNT(*) FROM outbox_events WHERE published_at IS NULL;
