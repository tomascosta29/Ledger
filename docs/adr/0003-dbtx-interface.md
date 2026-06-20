# ADR 0003: DBTX interface — repo methods work inside or outside a transaction

## Status

Accepted. 2026-06-20.

## Context

Annotation primitives (`Categorize`, `SetHidden`, `AddTag`, `RemoveTag`)
need to do three things atomically:

1. Write to the raw table (e.g. `UPDATE transactions SET ...`).
2. Write audit log entries.
3. Rebuild the overlay.

Per ADR 0002 the overlay rebuild is full and must happen inside the
same SQL transaction as the write. So the annotation service needs to
share a `*sql.Tx` across all three steps.

The existing repository methods take `context.Context` and use
`*sql.DB` internally. That means they implicitly open their own
transaction per call — which means a caller's `*sql.Tx` is bypassed.
The annotation service can't use the existing repos inside a tx
without duplicating every method.

## Decision

**Introduce a `ports.DBTX` interface that both `*sql.DB` and `*sql.Tx`
satisfy.** Repo write methods that need to participate in a tx take a
`DBTX` parameter; their existing non-tx versions are kept as thin
wrappers that pass `r.db`.

```go
type DBTX interface {
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
```

```go
func (r *TransactionRepository) SetCategory(ctx context.Context, id int64, category string) error {
    return r.SetCategoryDBTX(ctx, r.db, id, category)
}

func (r *TransactionRepository) SetCategoryDBTX(ctx context.Context, db ports.DBTX, id int64, category string) error {
    _, err := db.ExecContext(ctx, "UPDATE transactions SET category = ?, updated_at = ? WHERE id = ?",
        category, timeToISO(time.Now().UTC()), id)
    ...
}
```

Callers inside a tx pass the tx:

```go
tx, _ := s.db.BeginTx(ctx, nil)
defer tx.Rollback()
s.txRepo.SetCategoryDBTX(ctx, tx, id, category)
```

Callers outside a tx pass `r.db` (which satisfies `DBTX`):

```go
s.txRepo.SetCategory(ctx, id, category)  // equivalent to passing r.db
```

## Consequences

- **One method per write, two call sites.** Each write repo has
  `Method(ctx, ...)` (uses `r.db`) and `MethodDBTX(ctx, db, ...)` (uses
  passed DBTX). The non-DBTX version is a one-line wrapper.
- **Compile-time guarantee.** Both `*sql.DB` and `*sql.Tx` are
  statically asserted to satisfy `DBTX` via the `var _ DBTX = ...`
  declarations at the top of each repo file. A bug in either type
  fails the build, not at runtime.
- **The annotation service uses DBTX variants exclusively** so the
  tx is explicit at every call site. The non-DBTX variants stay for
  the import use case (which has its own internal transactions and
  doesn't need to share with the rebuild).
- **Read methods stay non-DBTX.** `GetByID`, `FindAll`, `Count` use
  `r.db` directly. They can be called inside or outside a tx; if a
  future caller needs them inside a tx, add DBTX variants on demand.

## Alternatives considered

- **Two parallel implementations (`SetCategory` and
  `SetCategoryWithTx(*sql.Tx)`)**. Rejected — duplicates SQL, easy to
  drift, harder to refactor.
- **Refactor all methods to take `*sql.Tx` only; the non-tx path opens
  its own.** Rejected — callers like the import use case would lose
  the option to commit per batch.
- **Closure-based helper (`s.executorFor(tx)` returns a function that
  binds to the tx).** Rejected — fun, but obscures the call site
  (every write becomes `s.exec(...)` instead of `s.repo.SetCategory(...)`).

## When this should be revisited

- If Go's `database/sql` adds a generic `sql.DBTX` (unlikely but
  possible), this ADR's interface becomes redundant and we delete it.
- If a future caller needs reads inside a tx, add `GetByIDDBTX` etc.
  Don't preemptively convert all reads.