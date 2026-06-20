# Architectural Decision Records

Decisions made during the v1 build of LedgerPro Go. Each ADR explains
the choice, the alternatives considered, and the conditions that
would cause us to revisit.

## Index

| ADR    | Topic                                       | Status   |
| ------ | ------------------------------------------- | -------- |
| 0001   | [SQLite connection strategy](./0001-sqlite-connection-strategy.md) | Accepted |
| 0002   | [Overlay rebuild strategy](./0002-overlay-rebuild-strategy.md)      | Accepted |
| 0003   | [DBTX interface for repo methods](./0003-dbtx-interface.md)         | Accepted |
| 0004   | [Tag storage as join table](./0004-tag-storage.md)                  | Accepted |

## When to write a new ADR

Write one when all three are true (per the domain-modeling skill):

1. **Hard to reverse** — the cost of changing your mind later is meaningful.
2. **Surprising without context** — a future reader will wonder "why did they do it this way?"
3. **The result of a real trade-off** — there were genuine alternatives and you picked one for specific reasons.

If any of the three is missing, skip the ADR. Keep the rationale in
the commit message and move on.