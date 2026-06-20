# LedgerPro Go

A hyperspecialized personal accounting tool for one operator. Single Go
binary, Bubble Tea TUI as the primary surface, cobra CLI as escape hatch.

This is a clean Go rewrite of the lessons-learnt TypeScript LedgerPro
project. See [SPEC.md](./SPEC.md) for the v1 plan and
[CONTEXT.md](./CONTEXT.md) for the domain vocabulary.

## Status

v0.0.0 — scaffolding. No domain code yet. See [SPEC.md](./SPEC.md) for the
build plan.

## Build

```sh
go build ./cmd/ledger
./ledger --help
```

Or install from source:

```sh
go install github.com/tomascosta29/Ledger/cmd/ledger@latest
```

## License

TBD.