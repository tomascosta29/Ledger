# ADR 0006: Unify Transfer and Reimbursement group types

## Status

Accepted. 2026-06-21.

## Context

`transaction_groups` currently has two types: `transfer` and `reimbursement`. A transfer is "money moved between the operator's own accounts" (net-zero); a reimbursement is "I spent X, someone paid me back X" (also net-zero in the overlay, but the original spend should be excluded from my spending count).

The structural distinction has three real costs:

1. **CLI surface is doubled**: `ledger transfers detect | confirm` and `ledger reimburse link` are two separate verbs for the same underlying operation (link two matching transactions into a group).
2. **Recipe complexity**: the recipe needs separate clauses for transfers and reimbursements, even though both are net-zero in the overlay and both need to be excluded.
3. **Detection asymmetry**: the transfer detector (heuristic: same absolute amount, opposite signs, 3-day window) is a separate code path from the manual reimbursement link, even though both are detecting the same shape (two matching transactions).

The semantic distinction is recoverable from the partner data: a transfer has both `partner_iban`s (or `partner_name`s) belonging to the operator; a reimbursement has at least one partner that's not the operator's. There is no first-class Account entity in v1, so "the operator's accounts" is implicit (whatever the operator recognizes by partner).

## Decision

**Unify Transfer and Reimbursement into a single group type.**

- **Schema**: drop the `type` column on `transaction_groups`. The group has 2+ members and that is all. Migration rewrites all existing rows to a single (or `NULL`) value.
- **Audit log**: keep `AuditActionTransferLink` and `AuditActionReimbursementLink` as distinct action constants, even though the group type is gone. The audit log records *what happened* (the operator linked a transfer or a reimbursement), and that historical distinction is worth preserving for replay. The action type is in the log, not on the row.
- **CLI**: `ledger transfers detect | confirm` is removed. `ledger reimburse link <id1> <id2>` becomes the only link verb. (Or a more neutral `ledger link` — operator's call.)
- **Detection**: `TransferService` and `ReimbursementService` merge into a single `GroupService.Detect` that produces candidates of the unified type. The heuristic (same absolute amount, opposite signs, 3-day window) is the same for both former types. Manual link is via the same verb.
- **Recipe**: the recipe clause becomes "exclude all groups" (i.e., exclude `source_kind='group'`). The previous "exclude reimbursements" clause is replaced.
- **Overlay**: `source_kind` values for groups become a single value (e.g., `'group'`). The previous `'transfer_group'` and `'reimbursement_group'` are merged. Migration rewrites the overlay.
- **TUI Linker screen**: shows all groups together. No separate "Transfers" / "Reimbursements" sections.

## Consequences

- **CLI and schema are simpler.** One verb, one type, one source_kind. Less code, fewer concepts to learn.
- **Audit log still preserves the action type.** Historical queries can still distinguish "I linked a transfer" from "I linked a reimbursement" by reading the log.
- **Recipe is simpler.** One exclusion clause covers both former types. The recipe author does not need to think about whether their exclude covers transfers.
- **Linker screen is denser.** All groups in one list. The operator infers transfer-vs-reimbursement from the partner names. For 50+ groups, this might feel cluttered — if so, a future iteration can add a derived label.
- **The "operator's accounts" question remains implicit.** There is no first-class Account in v1. The detection heuristic fires on same-amount opposite-sign pairs regardless of whose accounts they are; the operator confirms or rejects. The unification does not change this — the heuristic was always operating on the same shape.

## Alternatives considered

- **Keep both types.** Rejected — the semantic distinction is recoverable from partner data, and the doubled CLI / recipe / detection cost is real.
- **Auto-derive type from partner data at link time.** Considered — set `type` automatically based on whether the partner_ibans are recognized as the operator's. Rejected — there is no first-class Account, so "the operator's accounts" is implicit. The auto-derivation would need a config surface (which v1 does not have) or an in-band registration (which adds a new concept). Not worth it.
- **Drop the type from both the schema and the audit log.** Rejected — the audit log is the history; throwing away the action type loses information that might matter later (e.g. "did I link more transfers or more reimbursements this year?").

## When this should be revisited

- If the Linker screen feels too cluttered with mixed groups, add a derived `type` column (or filter) computed from partner data. The audit log can serve as ground truth if the derivation is ambiguous.
- If a first-class Account entity is ever introduced (currently a 🔮 v2 item), the transfer / reimbursement distinction becomes structurally meaningful: transfers are within the operator's account set, reimbursements are not. The `type` column could be re-introduced at that point. For v1.1, "unified" is the right shape.
- If recipes ever need to exclude transfers but not reimbursements (or vice versa), the recipe schema can grow an `is_own_account` clause (or similar). Not needed today.
