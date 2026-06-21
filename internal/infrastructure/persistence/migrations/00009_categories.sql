-- +goose Up

-- 1. New categories table.
CREATE TABLE categories (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    description TEXT    NOT NULL DEFAULT '',
    archived_at TEXT,
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_categories_name        ON categories(name);
CREATE INDEX idx_categories_archived_at ON categories(archived_at);

-- 2. Add nullable FK column on transactions.
ALTER TABLE transactions ADD COLUMN category_id INTEGER REFERENCES categories(id);

-- 3. Backfill: distinct non-Unknown values from TEXT column become rows.
--    Per ADR 0005 "Unknown" is a system state, not a value, so it is
--    not seeded as a row; existing txs with category='Unknown' will
--    end up with category_id = NULL (handled in step 4).
INSERT INTO categories (name)
SELECT DISTINCT category FROM transactions
 WHERE category IS NOT NULL
   AND category != ''
   AND category != 'Unknown';

-- 4. Set category_id on transactions by name join.
--    'Unknown' values map to NULL (uncategorized system state).
UPDATE transactions SET category_id = (
    SELECT id FROM categories WHERE name = transactions.category
)
WHERE category IS NOT NULL AND category != '' AND category != 'Unknown';

-- 5. Audit the backfilled categories as category_create events so
--    audit-log replay reproduces the categories table.
INSERT INTO audit_log (table_name, record_id, action, field, old_value, new_value)
SELECT 'categories', id, 'category_create', 'name', NULL, name
  FROM categories;

-- 6. Drop the old TEXT column and its index.
DROP INDEX IF EXISTS idx_transactions_category;
ALTER TABLE transactions DROP COLUMN category;

-- +goose Down

-- Recreate the category TEXT column.
ALTER TABLE transactions ADD COLUMN category TEXT NOT NULL DEFAULT 'Unknown';

-- Backfill from the FK.
UPDATE transactions SET category = (
    SELECT name FROM categories WHERE id = transactions.category_id
)
WHERE category_id IS NOT NULL;

-- Drop the FK column.
DROP INDEX IF EXISTS idx_transactions_category_id;
ALTER TABLE transactions DROP COLUMN category_id;

-- Remove the category_create audit rows so they don't replay into
-- a future categories table that won't match.
DELETE FROM audit_log WHERE table_name = 'categories' AND action = 'category_create';

DROP INDEX IF EXISTS idx_categories_archived_at;
DROP INDEX IF EXISTS idx_categories_name;
DROP TABLE IF EXISTS categories;
