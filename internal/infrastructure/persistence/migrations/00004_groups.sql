-- +goose Up
CREATE TABLE transaction_groups (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    type       TEXT    NOT NULL CHECK (type IN ('transfer', 'reimbursement')),
    name       TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE transaction_group_members (
    group_id       INTEGER NOT NULL REFERENCES transaction_groups(id) ON DELETE CASCADE,
    transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    role           TEXT    NOT NULL,
    PRIMARY KEY (group_id, transaction_id)
);

CREATE INDEX idx_group_members_txn ON transaction_group_members(transaction_id);

-- +goose Down
DROP INDEX IF EXISTS idx_group_members_txn;
DROP TABLE IF EXISTS transaction_group_members;
DROP TABLE IF EXISTS transaction_groups;