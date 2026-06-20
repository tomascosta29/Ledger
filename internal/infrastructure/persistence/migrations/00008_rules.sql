-- +goose Up
CREATE TABLE rules (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    name               TEXT    NOT NULL,
    priority           INTEGER NOT NULL DEFAULT 0,
    match_partner      TEXT,
    match_description  TEXT,
    match_amount_min   INTEGER,
    match_amount_max   INTEGER,
    set_category       TEXT,
    set_bucket_id      INTEGER REFERENCES buckets(id) ON DELETE SET NULL,
    add_tags           TEXT,
    enabled            INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at         TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at         TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_rules_priority ON rules(priority DESC, id);

-- +goose Down
DROP INDEX IF EXISTS idx_rules_priority;
DROP TABLE IF EXISTS rules;
