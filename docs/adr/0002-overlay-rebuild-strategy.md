# ADR 0002: Overlay rebuild strategy — full rebuild on every write

## Status

Accepted. 2026-06-20.

## Context

The overlay is the materialized read model for LedgerPro. Every
annotation write (categorize, tag, hide, split, group-link, undo) needs
to keep the overlay consistent with the raw layer. There are two
strategies:

- **Incremental**: only touch the affected overlay rows.
- **Full rebuild**: delete + reinsert the entire overlay.

The trade-off is write cost vs. correctness surface.

## Decision

**Full rebuild, inside the same SQL transaction as the write.** Every
annotation service does:

```go
uow.WithTx(func(tx *sql.Tx) error {
    if err := writeAnnotation(tx); err != nil { return err }
    return overlaySvc.RebuildWithTx(ctx, tx)
})
```

`OverlayService.RebuildWithTx` runs four SQL phases in order:

1. `DELETE FROM overlay_transactions`
2. Insert `split_header` rows (parents with children)
3. Insert `raw` rows (non-hidden, not a split parent, not in any group)
4. Insert `split_child` rows (resolve `parent_overlay_id` against step 2)
5. If `transaction_groups` exists: validate mixed currencies, then
   insert `transfer_group` + `reimbursement_group` synthetic rows

If any phase fails, the entire rebuild rolls back, and the caller's
write also rolls back (since they share the tx).

## Consequences

- **Always consistent**: any successful annotation write leaves the
  overlay in a coherent state. There's no "partial rebuild" failure
  mode.
- **Write cost is constant**: O(N) where N is the total transaction
  count, regardless of which row changed. At personal scale (~10k–100k
  rows), this is sub-second on SQLite.
- **No invalidation logic**: no per-row caches to bust, no per-feature
  invalidation graphs. Same code path for every annotation type.
- **Easy to debug**: "is the overlay correct?" is a single SQL query
  (`SELECT ... FROM overlay_transactions JOIN transactions ON ...`).
  Drift is impossible by construction; only operator-side manual SQL
  pokes can produce drift, and `ledger rebuild-overlay` recovers.

## Alternatives considered

- **Incremental rebuild**. Each annotation type computes which overlay
  rows to touch:
  - categorize → update 1 row
  - tag → update 1 row's tags column
  - hide → delete 1 row
  - split → insert header + N children, mark parent hidden
  - group link → delete N members, insert 1 synthetic
  - undo → rewind all of the above for the last batch
  This is fast (O(1) per write) but the bookkeeping is hairy: a single
  annotation might affect multiple overlay rows in non-obvious ways
  (e.g., split-children need to know their parent overlay id, which
  requires an extra SELECT). Easy to introduce bugs where some overlay
  rows drift out of sync. Rejected.

- **Trigger-based rebuild at SQL level**. SQLite triggers on
  `transactions` UPDATE/DELETE → rebuild the affected overlay row(s)
  via SQL. No Go orchestration. Rejected: trigger SQL is hard to
  reason about; debugging "why is the overlay stale" becomes "read the
  trigger source"; multiple-annotation writes compound the trigger
  logic.

- **Lazy refresh / periodic background rebuild**. Annotation writes
  mark dirty; a background process rebuilds. TUI shows stale data
  between rebuilds. Rejected: introduces moving parts and stale-data UX
  for negligible benefit.

## Operational notes

- **Overlay IDs grow monotonically across rebuilds.** Each rebuild
  reinserts rows, and SQLite `INTEGER PRIMARY KEY AUTOINCREMENT`
  increments from the highest-ever-used id. Three rebuilds of 174 rows
  produce IDs 1–522. Raw transaction IDs are unchanged. UI surfaces
  raw IDs, not overlay IDs, so this is cosmetic.
- **The rebuild is `*sql.Tx`-aware** (takes the caller's tx via
  `RebuildWithTx`) so it's atomic with annotation writes. The
  convenience `Rebuild(ctx)` opens its own tx for ad-hoc callers
  (e.g. `ledger rebuild-overlay`).
- **`transaction_groups` may not exist yet** when the rebuild runs
  (linker feature is v1 but lands later). The rebuild checks
  `sqlite_master` at runtime and skips group processing if absent,
  logging nothing. When migration 00004 introduces the empty tables,
  the rebuild starts emitting synthetic rows.

## When this should be revisited

- If transaction count exceeds ~500k, full-rebuild latency might
  become user-visible. Switch to incremental at that scale, but
  require a comprehensive test suite that proves incremental is
  bug-free.
- If we add features whose overlay derivation is dramatically more
  expensive than what we have today (e.g. expensive amortized
  schedules), full rebuild might be too slow even at small scale.
  Same trigger: switch to incremental.