-- +goose Up
CREATE TABLE transaction_tags (
    transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    tag            TEXT    NOT NULL,
    PRIMARY KEY (transaction_id, tag)
);

CREATE INDEX idx_transaction_tags_tag ON transaction_tags(tag);

-- +goose Down
DROP INDEX IF EXISTS idx_transaction_tags_tag;
DROP TABLE IF EXISTS transaction_tags;