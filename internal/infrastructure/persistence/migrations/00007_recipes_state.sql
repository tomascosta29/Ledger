-- +goose Up
CREATE TABLE recipes_state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS recipes_state;
