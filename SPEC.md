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
- ⏳ **Distribution:** GoReleaser + GitHub Releases (linux/darwin/windows,
  amd64+arm64). `go install github.com/tomascosta29/Ledger/cmd/ledger@latest`
  for devs. (Not yet wired — currently `go build` only.)

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

## 3. TUI ⏳

- ⏳ Bubble Tea + Bubbles + Lipgloss.
- ⏳ **Router model**: parent owns current screen + status bar; each screen is
  a self-contained `tea.Model`.
- ⏳ **5 screens**, navigated by `:screen N` or the screen key:
  1. **Manager** — transaction list, filter DSL (`desc:`, `partner:`,
     `iban:`, `min:`, `max:`, `sign:`).
  2. **Categorizer** — unknown transactions, bulk categorize,
     rule-create from focused tx.
  3. **Linker** — expense pane + reimbursement pane, link into persisted
     group.
  4. **Budget** — per-bucket allocation + spent vs remaining for selected
     period.
  5. **Recipes** — list / author / pick active recipe.
- ⏳ **Status bar** (always visible): DB path, active recipe, period, total
  tx count, mode badge (`NORMAL`/`INSERT`/`COMMAND`).
- ⏳ **Keybindings**: vim-style modal.
  - Normal: `j/k/g/G` nav, `/` filter, `x` toggle select, `:` command line,
    `?` help, `q` quit, `1..5` jump to screen.
  - Insert: triggered for text input. `Esc` returns to Normal.
  - Command: `:` enters. Subcommands: `cat`, `tag`, `hide`, `split`,
    `rule`, `recipe`, `screen N`, `quit`.
- ⏳ `?` opens a help overlay with all bindings + recipes screen summary.

## 4. Feature scope (v1)

| Feature                                                  | Status | Notes                                                                |
| -------------------------------------------------------- | ------ | -------------------------------------------------------------------- |
| CSV import (Erste + Revolut)                             | ✓      | `ledger import <file> --profile NAME [--dry-run]`                    |
| Manual `add`                                             | ✓      | CLI done; TUI modal ⏳                                               |
| Buckets (per-bucket allocation, assigned to txns)       | ✓      | `--bucket` on categorize; `ledger bucket list|create|...`; `ledger budget [--month]` |
| Bulk categorize / tag / hide                             | ✓      | Atomic across all ids; one undo reverts the whole batch; TUI screen ⏳ |
| Splits (parent/child)                                    | ⏳      | Schema + overlay support ✓; CLI + TUI ⏳                            |
| Rules (category+bucket+tags, priority, no overwrite)    | ⏳      | Author via Categorizer or CLI                                       |
| Reimbursement linker                                     | ⏳      | Persisted group, Linker screen                                      |
| Transfer detection (heuristic, persisted groups)        | ⏳      | Interactive confirm, `ledger transfers detect`                       |
| Summary recipes (include/exclude/amortize/net, TOML)    | ⏳      | Recipes screen + CLI flag                                            |
| Budget (per-bucket allocation, spent vs allocated)       | ✓      | `ledger budget [--month]`; Budget TUI screen ⏳                      |
| Undo (reverse-last-batch, atomic)                        | ✓      | Audit log captures every change; one method to write                |
| History (audit log viewer)                               | ✓      | `ledger history` command                                            |
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

```
ledger split <txID>                  # split into N children
ledger transfers detect              # heuristic transfer detection
ledger reimburse link                # manual group linking
ledger rule list|create|apply        # rules engine
ledger summary [--recipe R] [--month YYYY-MM]
ledger recipe list|show|use          # summary recipes
```

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