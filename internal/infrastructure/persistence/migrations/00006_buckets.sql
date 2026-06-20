-- +goose Up
CREATE TABLE buckets (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    name                     TEXT    NOT NULL UNIQUE,
    currency                 TEXT    NOT NULL,
    monthly_allocation_minor INTEGER NOT NULL,
    archived_at              TEXT,
    created_at               TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at               TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_buckets_archived ON buckets(archived_at);

ALTER TABLE transactions ADD COLUMN bucket_id INTEGER REFERENCES buckets(id) ON DELETE SET NULL;
CREATE INDEX idx_transactions_bucket ON transactions(bucket_id);

-- +goose Down
DROP INDEX IF EXISTS idx_transactions_bucket;
ALTER TABLE transactions DROP COLUMN bucket_id;
DROP INDEX IF EXISTS idx_buckets_archived;
DROP TABLE IF EXISTS buckets;
