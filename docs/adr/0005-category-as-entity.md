# ADR 0005: Promote Category from TEXT to first-class entity

## Status

Accepted. 2026-06-21.

## Context

Categories are the curated policy axis of classification — exactly one
per transaction, expected to be used in summary breakdowns and as
rule/recipe targets. They are *managed* (added, renamed, retired over
time) rather than freely extended.

The current implementation stores `transactions.category` as a TEXT
column. The "set of categories" is whatever values have ever been
written. This has three concrete problems:

1. **No rename primitive.** A category can only be changed by
   recategorizing every transaction that uses it, which produces N
   audit rows and a noisy trail.
2. **Typos are permanent.** A single typo (`wnat` instead of `want`)
   silently creates a new "category" that the system treats as real.
3. **No discovery.** The operator has no way to list the current
   categories, see how many transactions use each, or archive unused
   ones.

The audit log is the spine of the system — every visible state is
reproducible from raw transactions + audit log. Category metadata
(rename, create, archive) is therefore state that also belongs in the
audit trail, not just on the row.

## Decision

**Promote Category to a first-class entity.**

- New table `categories` (id, name, description, archived_at,
  created_at). Migration `00009_categories.sql`.
- `transactions.category_id` foreign key replaces
  `transactions.category` TEXT. The FK is nullable; `NULL` means
  "unknown" (not yet categorized). The TEXT column is dropped in the
  same migration.
- Migration backfill: distinct names from the existing TEXT column
  become rows in `categories`; `category_id` is set by name join.
- New audit actions: `category_create`, `category_rename`,
  `category_archive`. Recorded as
  `table_name="categories", record_id=<id>` events — the same per-row
  shape as per-transaction events, no schema change to `audit_log`.
- Undo switch learns the three new cases. `category_rename` undoes by
  restoring the prior name. `category_archive` undoes by clearing
  `archived_at`.
- New CLI surface: `ledger category list | create | rename | archive`.
- Rules (`set_category`) and recipes (`include` / `exclude` clauses)
  continue to reference category by name string. The service resolves
  to FK at apply time. Rename is transparent to rules and recipes —
  no change to their schemas.
- Overlay rebuild reads `category_id` from `transactions`, joins to
  `categories` for the name, and denormalizes `category_name` into
  the overlay for fast reads.
- The "uncategorize" verb is **deferred**. New imports default to
  `category_id = NULL`; there is no primitive to clear a category
  after the fact. If the operator needs it, add later.

## Consequences

- **"Unknown" is a system state, not a value.** The Categorizer screen
  filters on `category_id IS NULL`. The string `Unknown` is no longer
  magic and cannot be typed as a category value. A recipe cannot
  currently express "category is unset" as a condition; a future
  recipe-schema extension would address that.
- **Data migration is non-trivial.** Every existing transaction is
  touched (backfill, FK assignment, TEXT column drop). The migration
  must run inside a single SQL transaction with the overlay rebuild
  deferred until the migration is complete.
- **Rules and recipes are decoupled from the data model.** Renaming a
  category is a one-row update to `categories` plus a one-row audit
  event; rules and recipes that matched the old name now match the
  new name automatically.
- **Overlay denormalizes category name.** The overlay's
  `category_name` column is a snapshot of `categories.name` at rebuild
  time. If the operator wants to see the *historical* category name on
  an old transaction, they read the audit log, not the overlay.
- **Tag remains a join table (per ADR 0004).** The asymmetry is
  intentional: Tag is open-vocabulary and per-transaction; Category
  is curated and managed. They share annotation semantics but differ
  in lifecycle.

## Alternatives considered

- **TEXT + `category_metadata(name, description, archived_at)` table.**
  Keep `transactions.category` TEXT; add a parallel metadata table.
  Rejected — rename becomes `UPDATE category_metadata` + `UPDATE
  transactions` (N rows), producing N audit events. The "one event per
  rename" promise was the operator's hard requirement.
- **Defer until v2.** Rejected — the operator has already felt the
  pain: typos and the lack of a list primitive are real daily
  friction, not hypothetical. v1 is shipped; this is the next-round
  polish.
- **Promote Tag to a first-class entity too.** Rejected — Tag is
  open-vocabulary by design. Managing it as a curated set would
  defeat the purpose of the "filter on demand" axis.

## When this should be revisited

- If the operator ever wants to rename categories *in bulk* (e.g. "I
  want to call `want` `discretionary` and `savings` `investment`"),
  the rename primitive is per-category and they call it twice. A
  bulk-rename verb would be possible but probably not worth the
  complexity for one operator.
- If recipes need to express "category is unset" as a condition
  (summary queries over unknowns), extend the recipe schema with an
  `is_null` op. This is the natural follow-on.
- If the operator ever needs to *delete* (not archive) a category,
  the schema needs a soft-delete-or-cascade decision. Archive is
  enough for one operator.
