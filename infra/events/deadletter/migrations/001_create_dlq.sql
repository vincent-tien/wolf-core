-- +goose Up
CREATE TABLE IF NOT EXISTS dead_letter_queue (
    id          TEXT        PRIMARY KEY,
    subject     TEXT        NOT NULL,
    data        BYTEA,
    headers     JSONB,
    error       TEXT        NOT NULL DEFAULT '',
    attempts    INT         NOT NULL DEFAULT 0,
    original_at TIMESTAMPTZ,
    dead_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dlq_subject ON dead_letter_queue(subject);
CREATE INDEX IF NOT EXISTS idx_dlq_dead_at ON dead_letter_queue(dead_at);

-- +goose Down
DROP TABLE IF EXISTS dead_letter_queue;
