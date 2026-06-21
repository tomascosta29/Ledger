# LedgerPro Go — v1 Spec

A hyperspecialized personal accounting tool for one operator. Built as a
clean Go rewrite of the lessons-learnt TypeScript LedgerPro. Single static
binary, primary surface is a Bubble Tea TUI, with a cobra CLI as escape
hatch for ops and scripting.

This document is the source of truth for v1 scope. Anything not here is
v2 or later. For status of what's already shipped, see
[ROADMAP.md](./ROADMAP.md).

**Status legend:** ✓ Done · 🚧 In progress · ⏳ Next · 🔮 v2

---

## 1. Architecture ✓

- ✓ **Single Go binary** (`ledger`), distributed as a static binary.
- ✓ Subcommands for ops; `ledger tui` (or no args) opens the Bubble Tea TUI.
- ✓ **Module path:** `github.com/tomascosta29/Ledger`.
- ✓ **Repo layout:** `cmd/ledger/main.go` → `internal/{domain,application,infrastructure,tui,cli}`.
- ✓ **Go version:** 1.25.
- ✓ **Distribution:** GoReleaser + GitHub Releases (linux/darwin/windows,
  amd64+arm64). `go install github.com/tomascosta29/Ledger/cmd/ledger@latest`
  for devs. Workflow in `.github/workflows/release.yml`; config in
  `.goreleaser.yml`. CI in `.github/workflows/ci.yml`.

## 2. Persistence ✓

- ✓ Pure-Go SQLite via `modernc.org/sqlite` + `database/sql`.
- ✓ Schema migrations via `pressly/goose`. SQL files in
  `internal/infrastructure/persistence/migrations/`, versioned
  sequentially (currently 00001-00005).
- ✓ DB location: `$LEDGER_DB_PATH` env var or default
  `~/.local/share/ledger/ledger.db`.
- ✓ **No config file in v1.** No privacy mode. No first-class Account.
- ✓ **No FX / no multi-currency conversion.** Transactions carry native
  currency; summaries group per-currency.
- ✓ See [ADR 0001](./docs/adr/0001-sqlite-connection-strategy.md) for the
  connection-pool decision (MaxOpenConns=1).

## 3. TUI ✓

- ✓ Bubble Tea + Bubbles + Lipgloss.
- ✓ **Router model**: parent owns current screen + status bar; each screen is
  a self-contained `tea.Model`.
- ✓ **5 screens**, navigated by `1..5`:
  1. **Manager** — transaction list, filter DSL (`desc:`, `partner:`,
     `iban:`, `min:`, `max:`, `sign:`, `category:`, `bucket:`, `id:`).
     Bulk select (`x`/`X`), then `C` (categorize), `T` (tag), `H` (hide),
     `U` (undo).
  2. **Categorizer** — unknown transactions, in-place c (category) / b
     (bucket) / t (tag) keys with single-character prompts.
     Auto-reloads after each action.
  3. **Linker** — transfer candidates at the top, existing groups below;
     j/k to navigate, enter to confirm a candidate. Manual reimbursement
     link is via the CLI.
  4. **Budget** — per-bucket allocation + spent vs remaining for selected
     period. n/p ± month, T today, r reload.
  5. **Recipes** — list, j/k to navigate, `u` to set the focused recipe
     as active.
- ✓ **Status bar** (always visible): DB path, current screen, mode badge
  (`NORMAL` / `COMMAND` / `HELP`), transient status message.
- ✓ **Keybindings**: vim-style modal.
  - Normal: `j/k/g/G` nav, `/` filter, `x` toggle select, `?` help, `q`
    quit, `1..5` jump to screen.
  - Help: any key dismisses.
- ✓ `?` opens a help overlay with all bindings.

## 4. Feature scope (v1)

| Feature                                                  | Status | Notes                                                                |
| -------------------------------------------------------- | ------ | -------------------------------------------------------------------- |
| CSV import (Erste + Revolut)                             | ✓      | `ledger import <file> --profile NAME [--dry-run]`                    |
| Manual `add`                                             | ✓      | `ledger add --date D --amount A --currency C --description D ...`   |
| Buckets (per-bucket allocation, assigned to txns)       | ✓      | `--bucket` on categorize; `ledger bucket list|create|...`; `ledger budget [--month]` |
| Bulk categorize / tag / hide                             | ✓      | Atomic across all ids; one undo reverts the whole batch. TUI Manager: `x`/`X` select, `C`/`T`/`H`/`U` apply |
| Splits (parent/child)                                    | ✓      | `ledger split <txID> --child "amount|desc"` (interactive also). TUI: parent + children show in Manager / Budget |
| Rules (category+bucket+tags, priority, no overwrite)    | ✓      | `ledger rule list\|create\|delete\|apply`; `RuleService.Apply` walks enabled rules by priority |
| Reimbursement linker                                     | ✓      | `ledger reimburse link <expenseID> <reimbursementID>`; overlay shows the group as a single row with net amount |
| Transfer detection (heuristic, persisted groups)        | ✓      | `ledger transfers detect` + `confirm`; TUI Linker screen has candidates + groups |
| Summary recipes (include/exclude/amortize/net, TOML)    | ✓      | TUI + CLI done; amortize is v2 (TOML loads; service handles include / exclude / net) |
| Budget (per-bucket allocation, spent vs allocated)       | ✓      | `ledger budget [--month]`; TUI Budget screen with period nav         |
| Undo (reverse-last-batch, atomic)                        | ✓      | Audit log captures every change; one method to write                |
| History (audit log viewer)                               | ✓      | `ledger history [--tx-id N] [--action A] [--limit N]`                |
| Multi-currency grouping                                  | ✓      | No FX, no config                                                     |
| Overlay (materialized read model)                       | ✓      | See [ADR 0002](./docs/adr/0002-overlay-rebuild-strategy.md)        |
| Tag storage as join table                                | ✓      | See [ADR 0004](./docs/adr/0004-tag-storage.md)                      |
| DBTX interface for atomic writes                         | ✓      | See [ADR 0003](./docs/adr/0003-dbtx-interface.md)                    |
| Recurring detection                                      | 🔮     | v2                                                                   |
| Bills (declared schedules)                               | 🔮     | v2                                                                   |
| Explain                                                  | 🔮     | v2                                                                   |
| Doctor                                                   | 🔮     | v2                                                                   |
| Wizard / Shell / Convert / Export                        | 🔮     | v2                                                                   |

## 5. CLI v1 subcommand surface

### Done ✓

```
ledger init                          # create DB + run migrations
ledger import <file> --profile NAME  # CSV ingest (Erste, Revolut, custom TOML)
ledger add [--date D] --amount A --currency C --description D [--partner] [--iban] [--category]
ledger list [--limit N] [--category X] [--since YYYY-MM-DD]
ledger show <txID>                   # transaction detail
ledger history [--tx-id N] [--action A] [--limit N]
ledger rebuild-overlay               # full overlay rebuild (drift recovery)
ledger categorize <id1,id2,...> --category X [--bucket NAME]
ledger hide <id1,id2,...> [--unhide]
ledger tag <id1,id2,...> --add foo,bar [--remove baz]
ledger undo                          # reverse last batch
ledger bucket list|create|update|archive|delete
ledger budget [--month YYYY-MM]      # per-bucket allocation vs spend
ledger tui                           # placeholder; full TUI ⏳
```

### ⏳ Next

_None — all v1 subcommands are shipped. See [ROADMAP.md](./ROADMAP.md)._

## 6. Domain vocabulary

See [CONTEXT.md](./CONTEXT.md) for the canonical glossary. The audit
log is the spine: every annotation (categorize, tag, hide, split,
rule-apply, transfer-link, reimburse-link, undo) writes one or more
`audit_log` rows. Current state is reproducible from raw transactions
+ audit log + recipe definitions.

## 7. Migration from the TypeScript repo

✓ **None.** Start fresh. The TypeScript `LedgerPro` repo stays as the
lessons-learnt reference; the Go project begins with no data. The operator
re-imports CSVs into the new DB and rebuilds rules from scratch.

## 8. Architectural decisions

See [docs/adr/README.md](./docs/adr/README.md) for the index. Currently:

- [ADR 0001](./docs/adr/0001-sqlite-connection-strategy.md) — SQLite connection strategy (`MaxOpenConns=1`).
- [ADR 0002](./docs/adr/0002-overlay-rebuild-strategy.md) — Overlay rebuild (full on every write).
- [ADR 0003](./docs/adr/0003-dbtx-interface.md) — `DBTX` interface for atomic repo writes.
- [ADR 0004](./docs/adr/0004-tag-storage.md) — Tag storage as join table, denormalized into overlay.