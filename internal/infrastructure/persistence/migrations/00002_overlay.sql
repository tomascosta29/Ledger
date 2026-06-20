-- +goose Up
CREATE TABLE overlay_transactions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    effective_date      TEXT    NOT NULL,
    amount_minor        INTEGER NOT NULL,
    currency            TEXT    NOT NULL,
    description         TEXT    NOT NULL DEFAULT '',
    partner_name        TEXT,
    partner_iban        TEXT,

    category            TEXT    NOT NULL DEFAULT 'Unknown',
    bucket_id           INTEGER,
    tags                TEXT    NOT NULL DEFAULT '',

    parent_overlay_id   INTEGER REFERENCES overlay_transactions(id),
    group_id            INTEGER REFERENCES transaction_groups(id),
    group_role          TEXT,

    source_kind         TEXT    NOT NULL CHECK (source_kind IN (
        'raw',
        'split_child',
        'split_header',
        'transfer_group',
        'reimbursement_group'
    )),

    raw_transaction_id  INTEGER REFERENCES transactions(id),
    raw_transaction_ids TEXT,

    exclude_from_reports INTEGER NOT NULL DEFAULT 0 CHECK (exclude_from_reports IN (0, 1)),

    refreshed_at        TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_overlay_date        ON overlay_transactions(effective_date DESC);
CREATE INDEX idx_overlay_category    ON overlay_transactions(category);
CREATE INDEX idx_overlay_bucket      ON overlay_transactions(bucket_id);
CREATE INDEX idx_overlay_group       ON overlay_transactions(group_id);
CREATE INDEX idx_overlay_raw         ON overlay_transactions(raw_transaction_id);
CREATE INDEX idx_overlay_parent      ON overlay_transactions(parent_overlay_id);
CREATE INDEX idx_overlay_source_kind ON overlay_transactions(source_kind);

-- +goose Down
DROP INDEX IF EXISTS idx_overlay_source_kind;
DROP INDEX IF EXISTS idx_overlay_parent;
DROP INDEX IF EXISTS idx_overlay_raw;
DROP INDEX IF EXISTS idx_overlay_group;
DROP INDEX IF EXISTS idx_overlay_bucket;
DROP INDEX IF EXISTS idx_overlay_category;
DROP INDEX IF EXISTS idx_overlay_date;
DROP TABLE IF EXISTS overlay_transactions;