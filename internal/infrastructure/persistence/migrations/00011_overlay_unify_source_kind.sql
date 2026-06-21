-- +goose Up

-- Per ADR 0006: the `transfer_group` / `reimbursement_group` source_kind
-- values are unified into a single `group`. The historical action
-- constants (transfer_linked vs reimbursement_linked) preserve the
-- distinction in the audit log; the operator infers it from the
-- partner data when it matters.

-- Rewrite existing overlay rows so the new rebuild doesn't conflict.
UPDATE overlay_transactions SET source_kind = 'group'
 WHERE source_kind IN ('transfer_group', 'reimbursement_group');

-- Drop the old CHECK constraint and add a relaxed one that includes
-- 'group' instead of the two old values.
-- SQLite doesn't support ALTER CONSTRAINT, so recreate the table.
-- (goose wraps the migration in its own transaction.)

CREATE TABLE overlay_transactions_new (
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

    parent_overlay_id   INTEGER REFERENCES overlay_transactions_new(id),
    group_id            INTEGER REFERENCES transaction_groups(id),
    group_role          TEXT,

    source_kind         TEXT    NOT NULL CHECK (source_kind IN (
        'raw',
        'split_child',
        'split_header',
        'group'
    )),

    raw_transaction_id  INTEGER REFERENCES transactions(id),
    raw_transaction_ids TEXT,

    exclude_from_reports INTEGER NOT NULL DEFAULT 0 CHECK (exclude_from_reports IN (0, 1)),

    refreshed_at        TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
INSERT INTO overlay_transactions_new SELECT * FROM overlay_transactions;
DROP TABLE overlay_transactions;
ALTER TABLE overlay_transactions_new RENAME TO overlay_transactions;

CREATE INDEX idx_overlay_date        ON overlay_transactions(effective_date DESC);
CREATE INDEX idx_overlay_category    ON overlay_transactions(category);
CREATE INDEX idx_overlay_bucket      ON overlay_transactions(bucket_id);
CREATE INDEX idx_overlay_group       ON overlay_transactions(group_id);
CREATE INDEX idx_overlay_raw         ON overlay_transactions(raw_transaction_id);
CREATE INDEX idx_overlay_parent      ON overlay_transactions(parent_overlay_id);
CREATE INDEX idx_overlay_source_kind ON overlay_transactions(source_kind);

-- +goose Down

-- Map 'group' rows back to 'reimbursement_group' (the more conservative
-- restore — both old types were net-zero so either works for sums).
UPDATE overlay_transactions SET source_kind = 'reimbursement_group'
 WHERE source_kind = 'group';

CREATE TABLE overlay_transactions_old (
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

    parent_overlay_id   INTEGER REFERENCES overlay_transactions_old(id),
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
INSERT INTO overlay_transactions_old SELECT * FROM overlay_transactions;
DROP TABLE overlay_transactions;
ALTER TABLE overlay_transactions_old RENAME TO overlay_transactions;

CREATE INDEX idx_overlay_date        ON overlay_transactions(effective_date DESC);
CREATE INDEX idx_overlay_category    ON overlay_transactions(category);
CREATE INDEX idx_overlay_bucket      ON overlay_transactions(bucket_id);
CREATE INDEX idx_overlay_group       ON overlay_transactions(group_id);
CREATE INDEX idx_overlay_raw         ON overlay_transactions(raw_transaction_id);
CREATE INDEX idx_overlay_parent      ON overlay_transactions(parent_overlay_id);
CREATE INDEX idx_overlay_source_kind ON overlay_transactions(source_kind);
