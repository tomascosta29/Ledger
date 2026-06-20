-- +goose Up
ALTER TABLE transactions ADD COLUMN parent_transaction_id INTEGER REFERENCES transactions(id);

CREATE INDEX idx_transactions_parent ON transactions(parent_transaction_id);

-- +goose Down
DROP INDEX IF EXISTS idx_transactions_parent;
ALTER TABLE transactions DROP COLUMN parent_transaction_id;