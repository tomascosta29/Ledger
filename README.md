# LedgerPro Go

A hyperspecialized personal accounting tool for one operator. Single Go
binary, Bubble Tea TUI as the primary surface (when built), cobra CLI as
escape hatch for ops and scripting.

This is a clean Go rewrite of the lessons-learnt TypeScript LedgerPro.

## Reading order

1. **[SPEC.md](./SPEC.md)** — what v1 is and the architectural shape.
2. **[CONTEXT.md](./CONTEXT.md)** — the domain vocabulary (Operator, Raw Transaction, Annotation, Audit Trail, Summary Recipe, Tag, Category, Bucket, Reversal).
3. **[ROADMAP.md](./ROADMAP.md)** — what's shipped, what's next, what's deferred.
4. **[docs/adr/](./docs/adr/)** — written architectural decisions and why.

## Build

```sh
go build ./cmd/ledger
./ledger --help
```

Or install from source:

```sh
go install github.com/tomascosta29/Ledger/cmd/ledger@latest
```

## Try it

```sh
export LEDGER_DB_PATH=/tmp/ledger.db

# 1. Create the DB
./ledger init

# 2. Import a CSV statement (Erste or Revolut built-in; custom via
#    $LEDGER_PROFILE_DIR/mybank.toml)
./ledger import erste.csv --profile erste
./ledger import revolut.csv --profile revolut

# 3. See what landed
./ledger list --limit 20

# 4. Annotate
./ledger categorize 1 --category want
./ledger tag 1 --add rent,coffee
./ledger hide 2

# 5. Recover from drift (if you've poked the DB directly)
./ledger rebuild-overlay

# 6. Open the TUI (placeholder for now — full TUI in progress)
./ledger tui
```

## Status (v1 shipped)

All 18 v1 features are done and in `main`. See [ROADMAP.md](./ROADMAP.md)
for the full shipped list and [SPEC.md](./SPEC.md) §4 for the v1 contract.

- ✓ Persistence (SQLite + goose migrations)
- ✓ CSV import (Erste + Revolut + custom TOML profiles)
- ✓ Overlay (materialized read model, atomic rebuild on every annotation write)
- ✓ Annotation primitives: categorize / hide / tag / bulk + split + rules
- ✓ Audit trail + undo (reverse-last-batch, atomic)
- ✓ Buckets + budget
- ✓ TUI shell + 5 screens (Manager, Categorizer, Linker, Budget, Recipes)
- ✓ Splits + transfers + reimbursement linker
- ✓ Summary recipes (TOML, include/exclude/net) + summary command

🔮 v2 (after v1 is in real use): recurring detection, bills, explain,
doctor, wizard/shell/convert/export, recipe amortize, TUI command line,
privacy mode, first-class account, FX, config file.

## Tests

```sh
go test ./...
```

All packages have table-driven tests; the persistence and overlay
tests exercise round-trips and the atomic-rebuild invariant.

## Architecture

- `cmd/ledger/` — `main.go` (cobra root) + `util.go`
- `internal/domain/` — entities, value objects, errors
- `internal/application/` — services (overlay, annotation, import use case) + ports (interfaces)
- `internal/infrastructure/` — persistence (SQLite repos), CSV (profiles + parser)
- `internal/tui/` — Bubble Tea (placeholder; landing soon)
- `docs/adr/` — architectural decisions, in numbered order

## License

TBD.