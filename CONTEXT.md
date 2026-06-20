# LedgerPro Go

A hyperspecialized personal accounting tool for one operator. Single Go
binary, Bubble Tea TUI as the primary surface, cobra CLI as escape hatch.

See [SPEC.md](./SPEC.md) for the v1 plan and architecture. See
[docs/adr/](./docs/adr/) for architectural decisions.

## Domain vocabulary

**Operator**:
The sole human user of LedgerPro. All commands, screens, and reports are
designed for one person who knows the system intimately.
_Avoid_: User, customer, admin, owner

**Personal Accounting**:
The practice of recording, categorizing, and reconciling one's own
financial activity (transactions, transfers, bills, recurring items) over
time.
_Avoid_: Personal finance, wealth management, budgeting

**Hyperspecialized Tool**:
A tool optimized for a single operator and a single workflow.
General-purpose flexibility, multi-tenant features, and onboarding for
unfamiliar users are explicitly out of scope.
_Avoid_: General-purpose tool, multi-user tool

**Raw Transaction**:
An immutable record imported from a bank or other source. The original
amount, date, counterparty, and currency of a single financial event.
Never edited after import — only annotated.
_Avoid_: Entry, record, row

**Annotation**:
A classification, link, or other piece of derived state attached to a Raw
Transaction. Annotations are themselves written to the audit log so any
current view is traceable back to the events that produced it.
_Avoid_: Tag (when used as a synonym), metadata, label

**Audit Trail**:
The complete, append-only history of every change — annotations applied,
annotations changed, transactions hidden, rules added, transfers linked.
Every visible state is reproducible from Raw Transactions plus the Audit
Trail.

**Amortization (in queries)**:
Spreading a single transaction's amount across a defined period when
computing summaries, so that a quarterly bill doesn't appear to inflate
one month. A query-time transformation, not a stored state.
_Avoid_: Smoothed, allocated, distributed

**Summary Recipe**:
A small declarative composition of transformations — include, exclude,
amortize, net — that defines the lens for a monthly or periodic summary.
Recipes are query-time constructs; they do not mutate data. Stored as
TOML files in `~/.config/ledger/recipes/`.
_Avoid_: View, filter, report

**Tag**:
A fine-grained, free-form descriptor attached to a transaction. Multiple
tags per transaction are allowed. Examples: `rent`, `groceries`, `coffee`.
The "what specifically" axis of classification.
_Avoid_: Label, keyword

**Category**:
A small, fixed-ish set of policy classifications — exactly one per
transaction. The "why" axis: `need`, `want`, `savings`, plus operator
customs. Used to separate essentials from discretionary spend.
_Avoid_: Class, type

**Bucket**:
A high-level budget envelope scoped to a project or domain, with an
allocated amount. Exactly one per transaction (or zero). The "what project
does this fund" axis. Examples: `vacation-2026`, `apartment-reno`,
`new-laptop`. The level at which budgeting happens.
_Avoid_: Envelope, project

**Reversal**:
A `Reimbursement` group whose members are the same counterparty and
cancel in amount, so the original expense nets to zero in summaries.
Treated as `excluded` in the summary recipe rather than `offset`, since
the operator's intent is "this never should have counted as my spending."
Persisted as a group, not detected at query time.
_Avoid_: Bounce, chargeback, refund