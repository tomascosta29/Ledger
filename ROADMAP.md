# Roadmap

A live view of where LedgerPro Go is and where it's heading. For the
*what* and *why* of v1, see [SPEC.md](./SPEC.md). For domain
vocabulary, see [CONTEXT.md](./CONTEXT.md). For written architectural
decisions, see [docs/adr/](./docs/adr/).

## Status legend

- **✓ Done** — in main, tested, used end-to-end.
- **🚧 In progress** — actively being built (no item right now).
- **⏳ Next** — queued for the next round of work, ordered top-down.
- **🔮 v2** — deferred until v1 is in real use.

---

## ✓ Done

### Scaffolding
- [SPEC.md](./SPEC.md) — the v1 plan, written during grilling.
- [CONTEXT.md](./CONTEXT.md) — domain glossary (Operator, Raw Transaction, Annotation, Audit Trail, Summary Recipe, Tag, Category, Bucket, Reversal, etc.).
- Repo layout: `cmd/ledger/`, `internal/{domain,application,infrastructure,tui,cli}/`.
- Single-binary deployment: cobra subcommands + `ledger tui` placeholder.

### Persistence
- Pure-Go SQLite via `modernc.org/sqlite` + `database/sql`.
- Schema migrations via `pressly/goose`, embedded via `go:embed`.
- Migration `00001_init.sql`: `transactions`, `import_batches`, `audit_log`.
- Repos: `TransactionRepository`, `AuditLogRepository`, `ImportBatchRepository`, `TagRepository`, `OverlayRepository`.
- Value objects: `Money` (int64 minor units + currency), `IBAN` (mod-97 checksum).
- ADR 0001 — SQLite connection strategy (`MaxOpenConns(1)`).

### CSV import
- Data-driven `Profile` struct (column mapping + date format + decimal sep + state filter).
- Built-in profiles: Erste, Revolut. Custom profiles via `~/.config/ledger/profiles/*.toml`.
- Generic CSV parser (BOM stripping, EU/US thousands sep, lazy quotes, PENDING/REVERTED state filter).
- SHA-256 source hash for dedup.
- `ImportUseCase`: parse → dedup → insert → audit → update counts.
- CLI: `ledger import <file> --profile NAME [--dry-run]`.

### Overlay (materialized read model)
- Migration `00002_overlay.sql`: `overlay_transactions` table + 7 indexes.
- Migration `00003_splits.sql`: adds `transactions.parent_transaction_id` (needed for split detection in rebuild).
- Migration `00004_groups.sql`: empty `transaction_groups` + `transaction_group_members` (linker feature will populate).
- Migration `00005_tags.sql`: `transaction_tags` join table.
- `OverlayService.Rebuild(ctx)` + `RebuildWithTx(ctx, *sql.Tx)` — full rebuild in 4 phases (split_headers → raw → split_children → synthetic group rows).
- ADR 0002 — overlay rebuild strategy (full rebuild on every annotation write).
- `ledger list` reads from the overlay (not from raw transactions).
- `ledger rebuild-overlay` — manual rebuild for drift recovery.

### Annotation primitives
- `AnnotationService`: `Categorize`, `SetHidden`, `AddTag`, `RemoveTag`.
- Each verb is atomic: write → audit log → overlay rebuild, all in one `*sql.Tx`.
- DBTX interface (ADR 0003) — same repo method works inside or outside a tx.
- ADR 0004 — tag storage as join table (denormalized into overlay TEXT at rebuild time).
- CLI: `ledger categorize <id> --category X`, `ledger hide <id> [--unhide]`, `ledger tag <id> --add foo,bar [--remove baz]`.

### Audit trail
- Every annotation writes one or more `audit_log` rows (one per changed field, with old/new values).
- Idempotent on no-change (no audit row written).

### Undo
- `AnnotationService.Undo` — finds the last batch in `audit_log` (using timestamp grouping), reverses each entry, cleans up empty import batches, and rebuilds the overlay.
- CLI: `ledger undo` command.

### Bulk verb commands
- `AnnotationService.Bulk{Categorize,SetHidden,AddTags,RemoveTags}` — each runs in one `*sql.Tx`, writes N audit rows sharing a single timestamp, rebuilds the overlay once.
- CLI: `ledger categorize 1,2,3 --category want`, `ledger hide 1,2,3`, `ledger tag 1,2,3 --add foo --remove bar`. Single-id syntax still works (one entry is just a one-element batch).
- Atomic across all ids: a missing id rolls the whole batch back. `ledger undo` reverts every id in the batch in one step.

### Buckets
- Migration `00006_buckets.sql`: `buckets` table (name, currency, monthly_allocation_minor, archived_at) + `transactions.bucket_id` FK.
- `BucketRepository` (CRUD, archive, delete-with-assignment-check, `SpendByMonth`, `UnassignedSpendByMonth`).
- `AnnotationService.Categorize` / `BulkCategorize` accept an optional `*string` bucket name. If non-nil, the transaction is also assigned to the bucket; both writes share a single audit timestamp and a single overlay rebuild.
- Currency mismatch between bucket and transaction is rejected (a single bulk operation rejects if the selection spans multiple currencies).
- `AuditActionBucket = "bucket_assign"` — Undo handles it by restoring the prior bucket_id.
- CLI: `ledger bucket list|create|update|archive|delete` and `ledger budget [--month YYYY-MM]`.

### Manual add / Show / History
- `commands.ManualAddUseCase` — single-transaction entry outside the CSV import flow. Source hash is computed from the inputs (profile "manual" v1), so re-running with the same arguments is a no-op. Writes an `import` audit row, rebuilds the overlay. Bucket / category / partner / IBAN are all optional.
- `ledger add` (CLI).
- `ledger show <txID>` — full transaction detail incl. tags and bucket.
- `ledger history [--tx-id N] [--action A] [--limit N]` — audit log viewer.

### Splits (CLI)
- `commands.SplitUseCase` — splits a parent into N children whose amounts sum to the parent. Rejects re-splits, currency mismatches, and non-matching sums. Writes a single `split` audit row on the parent whose `NewValue` is the comma-separated child IDs.
- Undo handler for `split` action: parses the child ID list, deletes them, rebuilds overlay. The parent automatically re-appears as a raw row in the overlay because it no longer has children.
- Overlay rebuild now also excludes children from raw via `t.parent_transaction_id IS NULL` (the previous version only excluded parents that had children, which let children slip through as raw).
- `ledger list` includes `SourceSplitHeader` by default so a split shows up as a header + N children.
- `ledger split <txID> --child "amount|description"` (repeatable) for non-interactive, or no flags for interactive prompting.

### TUI shell
- Bubble Tea + Bubbles + Lipgloss wired in. The `App` is the root model: it owns the current screen, the status bar, and the help overlay. Status bar shows DB path, current screen title, mode badge, and a transient status slot. `?` opens the help overlay; any key dismisses. `1..5` jumps between the five screens; `q` / `ctrl+c` quits. Manager is the only screen with any real content so far — it loads the latest 200 overlay rows and supports `j/k/g/G` navigation. Categorizer / Linker / Budget / Recipes are stubs that document what each screen will own and tell the operator to use the CLI in the meantime. Subsequent milestones fill them in.

### Manager screen + filter DSL
- The TUI Manager screen supports the v1 filter DSL: `desc:`, `partner:`, `iban:`, `min:`, `max:`, `sign:`, `category:`, `bucket:`, `id:`. Clauses are whitespace-separated and AND-combined. A bare number is `min:<n>`. Press `/` to enter the filter input, `esc` to clear, `enter` to apply.
- Overlay repository extended with new filter fields: `PartnerName`, `PartnerIBAN`, `DescriptionLike`, `AmountMinMinor`, `AmountMaxMinor`, `AmountSign`, `BucketID`. SQL: `LIKE %...%` for substring match, exact for IBAN / category / id, `amount_minor >= / <= / < 0 / > 0` for the amount filters.
- `j/k/g/G/pgup/pgdown` for navigation. The status line shows the current count after a filter apply.

### Categorizer screen
- TUI screen that lists the v1 "Unknown" transactions (category = 'Unknown', source_kind = 'raw', newest first) and lets the operator annotate in place. j/k nav, plus three single-letter prompts:
  - `c` then type a category → categorize the focused row.
  - `b` then type a bucket name → assign bucket (current category preserved).
  - `t` then type a tag → add tag.
- Each action runs through the existing AnnotationService (single-tx atomic, audit row, overlay rebuild) and the screen auto-reloads. Status line shows the new "X unknown remaining" count after every action.
- Deps now carries the underlying `*sql.DB` so screens can build the AnnotationService that requires it for `BeginTx`.

### Budget screen
- TUI screen that mirrors the `ledger budget` CLI: per-bucket allocation vs spend, plus unassigned. Reads via the same BucketQuerier (SpendByMonth / UnassignedSpendByMonth) the CLI uses — no separate logic path.
- Keys: `n` / `p` step the month by ±1; `T` jumps to the current month; `r` reloads. The screen defaults to the current month on init.

### Recipes + Summary
- Migration `00007_recipes_state.sql`: a one-row `recipes_state(key, value)` for `key='active'` so the active recipe persists across runs.
- Recipes themselves are TOML files in `$LEDGER_RECIPES_DIR` (default `~/.config/ledger/recipes/*.toml`). Schema: `name`, `description`, `include = [{field, op, value}, ...]`, `exclude = [...]`, `net = bool`. Supported fields: `category`, `partner`, `bucket`, `tag`. Supported ops: `is`, `not`, `contains`.
- `RecipeRepository` (TOML-on-disk + state-in-DB), `SummaryService` (apply recipe to overlay rows for a month, group by currency, separate income / expense unless `net=true`).
- CLI: `ledger recipe list|show|use|new` and `ledger summary [--recipe NAME] [--month YYYY-MM]` (defaults to active recipe + current month).
- TUI: Recipes screen — j/k to navigate, `u` to set the focused recipe as active.

### Rules engine
- Migration `00008_rules.sql`: `rules` table with `name`, `priority`, matchers (`match_partner`, `match_description`, `match_amount_min`, `match_amount_max`), effects (`set_category`, `set_bucket_id`, `add_tags`), `enabled` flag. All fields are optional except `name` and `enabled`; null matchers are skipped.
- `RuleService.Apply()` walks enabled rules in priority order, scans every transaction, calls the existing `AnnotationService` for each match. "No overwrite" semantics: only sets `category` if currently "Unknown", only sets `bucket` if currently `null`, only adds tags that aren't already present. All writes go through AnnotationService so they're atomic and audited; the existing `undo` reverses per-application-batch.
- CLI: `ledger rule list|create|delete|apply`. `apply` prints "X matched, Y applied, Z skipped" plus per-rule counts.

---

## ⏳ Next (priority order)

1. **Transfer detection + Linker** — `TransferDetectionService`, `ledger transfers detect`, Linker TUI screen for manual linking. ~2-3 days.
2. **Manager bulk actions** — `x` to toggle select, `:` for command line (cat/tag/hide/split/undo on the selection). ~half day.

This order is approximate — exact ordering depends on what the
operator wants to drive daily. Buckets before rules because the Budget
screen is a more compelling daily-driver than rule authoring.

---

## 🔮 v2 (deferred until v1 ships and is in real use)

From [SPEC.md](./SPEC.md) Section 4:

- **Recurring detection** — backward-looking: "this looks like a subscription."
- **Bills (declared schedules)** — forward-looking: "rent is €1200 on the 1st."
- **Explain** — "why did rule X match transaction Y?"
- **Doctor** — data health checks (drift detection on the overlay is one of these).
- **Wizard / Shell / Convert / Export** — convenience features that are easy to add later but not blocking the daily loop.

From SPEC Section 2:

- **Privacy mode** — operator trusts their own machine for v1.
- **First-class Account** — IBAN-as-account or account-id; not blocking any v1 screen.
- **FX / multi-currency conversion** — per-currency grouping is enough for v1.
- **Config file** — env vars + flags only in v1.

---

## Architectural decisions

| ADR    | Topic                                      | Status   |
| ------ | ------------------------------------------ | -------- |
| 0001   | SQLite connection strategy                 | Accepted |
| 0002   | Overlay rebuild strategy (full on write)   | Accepted |
| 0003   | DBTX interface for repo methods            | Accepted |
| 0004   | Tag storage as join table                  | Accepted |

When a future contributor asks "why was it done this way?", start
with the ADR index in [docs/adr/README.md](./docs/adr/README.md).