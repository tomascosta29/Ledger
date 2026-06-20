# LedgerPro Go — v1 Spec

A hyperspecialized personal accounting tool for one operator. Built as a
clean Go rewrite of the lessons-learnt TypeScript LedgerPro. Single static
binary, primary surface is a Bubble Tea TUI, with a cobra CLI as escape
hatch for ops and scripting.

This document is the source of truth for v1 scope. Anything not here is
v2 or later.

---

## 1. Architecture

- **Single Go binary** (`ledger`), distributed as a static binary.
- Subcommands for ops; `ledger tui` (or no args) opens the Bubble Tea TUI.
- **Module path:** `github.com/tomascosta29/Ledger`.
- **Repo layout:** `cmd/ledger/main.go` → `internal/{domain,application,infrastructure,tui,cli}`.
- **Go version:** 1.25.
- **Distribution:** GoReleaser + GitHub Releases (linux/darwin/windows,
  amd64+arm64). `go install github.com/tomascosta29/Ledger/cmd/ledger@latest`
  for devs.

## 2. Persistence

- Pure-Go SQLite via `modernc.org/sqlite` + `database/sql`.
- Schema migrations via `pressly/goose`. SQL files in
  `internal/infrastructure/persistence/migrations/`, versioned
  sequentially.
- DB location: `$LEDGER_DB_PATH` env var or default
  `~/.local/share/ledger/ledger.db`.
- **No config file in v1.** No privacy mode. No first-class Account.
- **No FX / no multi-currency conversion.** Transactions carry native
  currency; summaries group per-currency.

## 3. TUI

- Bubble Tea + Bubbles + Lipgloss.
- **Router model**: parent owns current screen + status bar; each screen is
  a self-contained `tea.Model`.
- **5 screens**, navigated by `:screen N` or the screen key:
  1. **Manager** — transaction list, filter DSL (`desc:`, `partner:`,
     `iban:`, `min:`, `max:`, `sign:`).
  2. **Categorizer** — unknown transactions, bulk categorize,
     rule-create from focused tx.
  3. **Linker** — expense pane + reimbursement pane, link into persisted
     group.
  4. **Budget** — per-bucket allocation + spent vs remaining for selected
     period.
  5. **Recipes** — list / author / pick active recipe.
- **Status bar** (always visible): DB path, active recipe, period, total
  tx count, mode badge (`NORMAL`/`INSERT`/`COMMAND`).
- **Keybindings**: vim-style modal.
  - Normal: `j/k/g/G` nav, `/` filter, `x` toggle select, `:` command line,
    `?` help, `q` quit, `1..5` jump to screen.
  - Insert: triggered for text input. `Esc` returns to Normal.
  - Command: `:` enters. Subcommands: `cat`, `tag`, `hide`, `split`,
    `rule`, `recipe`, `screen N`, `quit`.
- `?` opens a help overlay with all bindings + recipes screen summary.

## 4. Feature scope (v1)

| Feature                                | v1 | Notes                                                                       |
| -------------------------------------- | -- | --------------------------------------------------------------------------- |
| CSV import (Erste + Revolut)           | ✓  | TUI import wizard: pick file → preview → confirm                            |
| Manual `add`                           | ✓  | CLI subcommand + TUI modal                                                  |
| Bulk categorize / tag / hide           | ✓  | CLI + Categorizer TUI screen                                                |
| Splits (parent/child)                  | ✓  | Port current model                                                          |
| Rules (category+bucket+tags, priority, no overwrite) | ✓ | Author via Categorizer or CLI                                          |
| Reimbursement linker                   | ✓  | Persisted group, Linker screen                                              |
| Transfer detection (heuristic, persisted groups) | ✓ | Interactive confirm, `ledger transfers detect`                          |
| Summary recipes (include/exclude/amortize/net, TOML files) | ✓ | Recipes screen + CLI flag                                            |
| Budget (per-bucket allocation, spent vs allocated) | ✓ | Budget screen                                                           |
| Undo + History (audit log)             | ✓  | Undo is reverse-last-annotation-batch                                       |
| Multi-currency grouping                | ✓  | No FX, no config                                                            |
| Recurring detection                    | ✗  | v2                                                                          |
| Bills (declared schedules)             | ✗  | v2                                                                          |
| Explain                                | ✗  | v2                                                                          |
| Doctor                                 | ✗  | v2                                                                          |
| Wizard / Shell / Convert / Export      | ✗  | v2                                                                          |

## 5. CLI v1 subcommand surface

```
ledger init                    # create DB + run migrations
ledger import <file>           # CSV ingest (Erste + Revolut)
ledger add                     # manual transaction entry
ledger list [--filter DSL]     # transaction list (read-only)
ledger show <id>               # transaction detail
ledger categorize <ids...>     # bulk category + bucket + tags
ledger tag <ids...> --add --remove
ledger hide <ids...> / unhide <ids...>
ledger split <id>              # split a transaction
ledger rule list|create|apply
ledger transfers detect        # interactive transfer detection
ledger reimburse link          # manual linking (CLI form of Linker)
ledger summary [--recipe R] [--month YYYY-MM]
ledger budget [--month YYYY-MM]
ledger undo
ledger history [--tx-id N]
ledger tui                     # open Bubble Tea TUI
```

## 6. Domain vocabulary

See `CONTEXT.md` for the canonical glossary. The audit log is the spine:
every annotation (categorize, tag, hide, split, rule-apply, transfer-link,
reimburse-link, undo) writes one or more `audit_log` rows. Current state
is reproducible from raw transactions + audit log + recipe definitions.

## 7. Migration from the TypeScript repo

**None.** Start fresh. The TypeScript `LedgerPro` repo stays as the
lessons-learnt reference; the Go project begins with no data. The operator
re-imports CSVs into the new DB and rebuilds rules from scratch.