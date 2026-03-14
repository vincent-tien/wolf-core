-- name: InsertInboxEvent :execrows
INSERT INTO inbox (event_id, event_type, consumer_group, processed_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (event_id, consumer_group) DO NOTHING;
