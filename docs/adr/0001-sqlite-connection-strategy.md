# ADR 0001: SQLite connection strategy — `MaxOpenConns(1)` for the entire process

## Status

Accepted. 2026-06-20.

## Context

We chose pure-Go SQLite via `modernc.org/sqlite` as the v1 persistence
engine (see SPEC.md). `database/sql` defaults to unlimited connections,
which on SQLite creates real problems:

- SQLite serializes writes anyway. Multiple connections don't give
  parallelism — they give the appearance of parallelism plus extra
  serialization points (one transaction per connection).
- WAL mode is enabled, but `busy_timeout` is the only safety net. With
  N connections, the Nth writer hits `SQLITE_BUSY` even within the same
  process.
- During the v1 build, `AuditLogRepository.LastBatch` opened two
  `QueryContext` calls back-to-back on the same `*sql.DB`. With the
  default unlimited pool the first call's open rows held a connection,
  and the second call's `database/sql.(*DB).conn` blocked forever in the
  test. That was the bug that motivated this ADR.

## Decision

**Pin `MaxOpenConns(1)` (and `MaxIdleConns(1)`) on the `*sql.DB`
returned from `persistence.Open`.** Every code path that talks to the
database goes through this single connection, so concurrent calls
serialize predictably instead of deadlocking on row handles.

Consequences:

- All repository methods must consume their `*sql.Rows` fully before
  starting another query. Multi-statement logic that needs two open
  result sets must use a `*sql.Conn` (acquired via
  `db.Conn(ctx)`) so both queries share one pooled connection, or must
  be rewritten as a single SQL statement (what we did for `LastBatch`).
- `database/sql` keeps the single connection alive for the process
  lifetime. No pooling, no fairness, no surprise contention.
- Bench throughput for bulk imports is limited by SQLite's single-writer
  throughput. For the operator's dataset (years of personal transactions,
  tens of thousands of rows) this is irrelevant — measured microseconds.

## Alternatives considered

- **`MaxOpenConns(N)` with N > 1.** Lets different repo methods run in
  parallel, but SQLite doesn't reward that and the row-handle-leak class
  of bugs returns. Rejected.
- **Per-call `*sql.Conn` acquisition.** Acquire and release a connection
  per method. Lets the driver pool, but reintroduces the same contention
  if two methods run concurrently. Rejected — no upside over pinning at
  one.
- **Switch off `database/sql` and use the raw `*sqlite.DB` driver.**
  More direct, no pool wrapper, but loses `sql.DB`'s context-aware
  `QueryContext` ergonomics. Worth revisiting only if we hit a perf wall.

## Consequences for later code

- Anyone writing a repo method must read rows to exhaustion before
  issuing another query, or use `db.Conn(ctx)` to pin a connection.
- Anyone opening multiple transactions at once will deadlock. Multi-step
  writes belong inside `UnitOfWork.WithTx`, which gives them a single
  `*sql.Tx` and therefore a single connection.
- Tests should exercise multi-query code paths explicitly. The
  persistence test suite already has `TestAuditLogRoundTrip` that would
  have caught the original bug.