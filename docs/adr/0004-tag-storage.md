# ADR 0004: Tag storage as a join table, denormalized into the overlay

## Status

Accepted. 2026-06-20.

## Context

Tags are a fine-grained, free-form descriptor attached to a
transaction. Multiple tags per transaction are allowed. Examples:
`rent`, `groceries`, `coffee`.

The original TypeScript LedgerPro stored tags as a comma-separated
string in `transactions.tags TEXT`. That's query-hostile: "find all
transactions tagged `rent`" requires `WHERE ',' || tags || ',' LIKE
'%,rent,%'` to avoid false positives (`rental`, `parent`).

The Go port is a clean rewrite. We can pick the right shape now.

Three options were considered:

1. **Comma-separated TEXT on `transactions`** — matches the TS design.
   Query-hostile, but simple.
2. **JSON array on `transactions`** — slightly more flexible,
   SQLite JSON1 functions can query, but JSON in SQLite is a smell.
3. **Join table `transaction_tags(transaction_id, tag)`** —
   normalized, indexable, no false positives.

## Decision

**Join table.** `transaction_tags` with `(transaction_id, tag)` and a
primary key on the pair; an index on `tag` for the common
"transactions tagged X" query.

The overlay's `tags TEXT` column stores the tags as a comma-separated
string (matching the rest of the denormalized read model). The
overlay rebuild reads from `transaction_tags` via
`GROUP_CONCAT(tt.tag, ',')` and writes the result into the overlay row
in one INSERT...SELECT.

Tag mutations (add/remove) write to `transaction_tags`, write an
audit log entry, and rebuild the overlay — atomic per
[ADR 0002](./0002-overlay-rebuild-strategy.md).

## Consequences

- **Tag queries are cheap and correct.** `SELECT * FROM transaction_tags WHERE tag = 'rent'` is a single index seek. No LIKE, no JSON parsing.
- **Tags are normalized at write time.** The TagRepository lowercases
  and trims tags (`normalizeTag`), so "Rent" and " rent " are the same
  tag.
- **Audit log captures tag diffs.** The audit row for an `AddTag` has
  `old_value` = comma-separated previous tags, `new_value` = new
  comma-separated tag set. The audit log entry describes what the
  tags were at write time, not the per-tag delta. That's the same
  granularity we use for other fields.
- **Overlay rebuild reads from `transaction_tags`**, not from a
  cached value on `transactions`. This means tag changes are picked
  up by the next rebuild without needing an additional "tags
  changed" signal.
- **Storage cost.** Two rows per tag instead of one comma-separated
  field. For personal-scale data (hundreds of tags), this is
  negligible.

## Alternatives considered

- **Comma-separated TEXT, ported 1:1 from TS.** Rejected: we already
  decided this was bad at the original grilling session
  (Q2 way back). The TS code has this; we don't have to repeat it.
- **JSON array on `transactions`.** Rejected: makes tag queries
  harder, not easier, and "JSON in a relational column" is a smell
  we want to avoid at the schema level even if SQLite supports it.
- **First-class `tags` table with tag definitions + join.** Rejected:
  more complex (a tag definition, a `transaction_tag` join, all
  inserts through both) for no real benefit. Tags are free-form;
  the operator doesn't curate them.

## When this should be revisited

- If the operator wants tag metadata (color, icon, description, "is
  this a recurring tag"), the schema needs to grow a `tags`
  definition table and a join with metadata. Defer until asked for.
- If a query needs to know "every tag ever attached to any
  transaction" (for autocomplete), the join table already supports
  it via `SELECT DISTINCT tag FROM transaction_tags ORDER BY tag`. No
  schema change needed.