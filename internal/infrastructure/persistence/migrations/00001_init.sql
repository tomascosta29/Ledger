-- +goose Up
CREATE TABLE import_batches (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    source_file     TEXT NOT NULL,
    source_profile  TEXT NOT NULL,
    row_count       INTEGER NOT NULL DEFAULT 0,
    inserted_count  INTEGER NOT NULL DEFAULT 0,
    skipped_count   INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE transactions (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    effective_date       TEXT    NOT NULL,
    amount_minor         INTEGER NOT NULL,
    currency             TEXT    NOT NULL,
    description          TEXT    NOT NULL DEFAULT '',
    partner_name         TEXT,
    partner_iban         TEXT,
    import_batch_id      INTEGER REFERENCES import_batches(id) ON DELETE SET NULL,
    source_hash          TEXT    NOT NULL,
    raw_data             TEXT,
    raw_description      TEXT,
    category             TEXT    NOT NULL DEFAULT 'Unknown',
    exclude_from_reports INTEGER NOT NULL DEFAULT 0 CHECK (exclude_from_reports IN (0, 1)),
    is_hidden            INTEGER NOT NULL DEFAULT 0 CHECK (is_hidden IN (0, 1)),
    created_at           TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at           TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_transactions_effective_date ON transactions(effective_date DESC);
CREATE INDEX idx_transactions_source_hash    ON transactions(source_hash);
CREATE INDEX idx_transactions_import_batch   ON transactions(import_batch_id);
CREATE INDEX idx_transactions_is_hidden      ON transactions(is_hidden);
CREATE INDEX idx_transactions_category       ON transactions(category);

CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    table_name  TEXT    NOT NULL,
    record_id   INTEGER NOT NULL,
    action      TEXT    NOT NULL,
    field       TEXT,
    old_value   TEXT,
    new_value   TEXT,
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_audit_log_record   ON audit_log(table_name, record_id, created_at DESC);
CREATE INDEX idx_audit_log_created  ON audit_log(created_at DESC);
CREATE INDEX idx_audit_log_action   ON audit_log(action);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_log_action;
DROP INDEX IF EXISTS idx_audit_log_created;
DROP INDEX IF EXISTS idx_audit_log_record;
DROP TABLE IF EXISTS audit_log;

DROP INDEX IF EXISTS idx_transactions_category;
DROP INDEX IF EXISTS idx_transactions_is_hidden;
DROP INDEX IF EXISTS idx_transactions_import_batch;
DROP INDEX IF EXISTS idx_transactions_source_hash;
DROP INDEX IF EXISTS idx_transactions_effective_date;
DROP TABLE IF EXISTS transactions;

DROP TABLE IF EXISTS import_batches;