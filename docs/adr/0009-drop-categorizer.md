# ADR 0009: Drop Categorizer screen — Manager absorbs single-row annotation

## Status

Accepted. 2026-06-21.

## Context

After v1.2 shipped the chrome + visual polish, the operator
flagged that **Manager and Categorizer were near-duplicates**:
same row shape, same chrome, same status bar — only the footer
keys and the pre-filter to Unknown distinguished them. Two
screens with the same data, same rendering, and overlapping
keymaps (j/k nav + category input) cost more to learn and
maintain than they paid back.

The Categorizer's value was a tighter triage loop:

1. Rows are always Unknown (`category_id IS NULL`).
2. Single-row annotation: `c` cat / `b` bucket / `t` tag + `Enter`.
3. No filter DSL needed.

Manager has bulk annotation (`x` to select, `C cat` for the
selection) and the full filter DSL, but no way to annotate a
single row in place. Adding it closes the gap.

The trade-off: one fewer screen, one less mental model. The
trade-off against: the bulk path becomes slightly longer (no
"annotation tab" to switch to) and the Manager screen does more
things (more keys).

## Decision

**Drop the Categorizer screen. Manager gains its responsibilities.**

### Manager gains

- Single-row annotation: `c` / `b` / `t` open an input prompt on
  the cursor row. `Enter` applies, `Esc` cancels, runes and
  backspace edit. The prompt replaces the body and the footer
  swaps to `Input: <verb>` + `Enter apply · Esc cancel · Bksp edit`.
- Bulk annotation: `C` / `B` / `T` open the same input prompt but
  on the multi-selection. Footer says `Bulk: <verb>`.
- Jump to next Unknown: `n` moves the cursor to the next row with
  empty `category`. Wraps once; status reports "no Unknown rows".
- Link selected: `l` runs transfer detection on the selection and
  confirms any pair where both the out-tx and in-tx are in the
  selection. Status reports pairs linked.
- `B` bulk bucket: previously the only annotation verbs on the
  selection were `C` and `T`. `B` makes bucket symmetric.

### Service layer

- New `AnnotationService.SetBucket(ctx, txID, bucketName)` and
  `BulkSetBucket(ctx, ids, bucketName)`. The existing
  `Categorize(ctx, txID, category, bucketName)` requires
  `category != ""` and would fail on Unknown rows. Bucket-only
  annotation is a real workflow ("this is the rent bucket, the
  category will come later") and needs its own audit action —
  `AuditActionBucket` (`"bucket_assign"`). The existing Undo
  handler already supports `AuditActionBucket` (parses the old
  bucket ID from `OldValue`, restores via `SetBucketDBTX`), so no
  service-layer undo work was needed.

### Sidebar drops from 5 to 4

```
► Manager   (was 1; absorbs Categorizer)
  Linker    (was 3; now 2)
  Budget    (was 4; now 3)
  Recipes   (was 5; now 4)
```

Number keys `1`–`4`. The help overlay is updated: the
Categorizer section is removed; the Manager section gains the new
keys.

### Footer keys (Normal, nothing selected)

Ordered by importance — narrow terminals truncate from the right.

```
j/k nav · n unk · c cat · t tag · b bkt · l link · / filter · x select ·
C cat·N · T tag·N · H hide · U undo · ? help
```

Single-row keys come first (primary triage path). Bulk keys come
later and drop first at narrow widths. The selection footer
(matching pairs with `Count: N` badges) replaces this when
anything is selected.

## Consequences

- **One fewer screen.** Sidebar shows 4 entries; number keys are
  `1`–`4`.
- **Manager does both single-row and bulk annotation.** The
  operator chooses per-row keystroke cost:
  `c food Enter` for one row, or `x x x C food Enter` for many.
- **Triage loop recovered.** `n` jumps to next Unknown without
  typing a filter, so the Categorizer workflow
  (import → annotate → next → annotate → next) still works.
- **Bucket annotation now reachable from Manager.** Previously
  Categorizer had `b` but Manager didn't. Now Manager has both
  `b` (cursor row) and `B` (selection).
- **No schema changes.** No new migrations. No new dependencies.
- **Service API grew.** `SetBucket` / `BulkSetBucket` are
  additive; nothing existing was renamed or removed.

## Alternatives considered

- **Keep both screens, add single-row keys to Manager anyway.**
  Rejected — duplicates the data view for no additional
  capability. The pre-filter to Unknown is the only real
  Categorizer affordance, and `n` plus the existing `/` filter
  cover the same need.
- **Drop Manager, keep Categorizer with the filter DSL grafted
  on.** Rejected — Categorizer is structurally narrower (no
  selection, no bulk), so making it the universal screen means
  adding the bulk path *and* losing the always-everything default.
  More new code than just absorbing into Manager.
- **Add a `: command` line on Manager for typed annotation.**
  Deferred — the `:` command is in the help overlay but not yet
  implemented anywhere. It's a separate, larger feature
  (typed-command parser, history, completion). The `c` / `b` /
  `t` keys are the minimum viable replacement.

## When this should be revisited

- If the operator's annotation pattern shifts heavily to "type
  multi-word category names" (e.g., recurring-tag workflows),
  the single-letter verbs become awkward. The `: command` line
  is the right next step — typed commands can include the
  category name directly (`c groceries` instead of `c` then
  `groceries Enter`).
- If a sixth screen is ever added (e.g., Bills), the sidebar
  layout may need a different organization (collapse, groups,
  sub-menus). The 5-screen assumption is gone now that there are
  4, but the layout still assumes a flat list.
