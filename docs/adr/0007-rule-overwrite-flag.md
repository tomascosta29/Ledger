# ADR 0007: Rule apply --overwrite flag for bulk-fix

## Status

Accepted. 2026-06-21.

## Context

The RuleService implements "no overwrite" semantics: a rule only sets
category if the transaction's category is currently `"Unknown"`
(`internal/application/services/rule.go:103`), only sets bucket if
`BucketID` is `nil` (line 110), and only adds tags not already present
(line 124). This preserves information: a non-Unknown transaction was
set by hand or by an earlier rule.

The friction case: the operator writes a rule that mistakenly sets the
wrong category (e.g. `rent` rule sets `want` instead of `need`). They
want to fix all matching transactions without manually recategorizing
each one. With "no overwrite," this requires N manual `categorize`
commands or N `bulk-categorize` calls.

The trade-off: relax the "no overwrite" check, allow the operator to
opt into overwriting on a per-`apply` basis. The audit log captures
every change, so the chain of overwrites is recoverable.

## Decision

**Add a `--overwrite` flag to `ledger rule apply`.**

- **CLI**: `ledger rule apply [--overwrite]`. Default behavior is
  unchanged (no-overwrite).
- The flag is per-run, not per-rule. The operator decides when they
  want overwrite semantics.
- `RuleService.Apply` accepts an `overwrite bool` parameter. When
  `overwrite=true`, the no-overwrite checks at lines 103, 110, 124
  are bypassed.
- **New audit action**: `AuditActionRuleApply`. When the flag is
  set, the annotation service writes `RuleApply` rows instead of
  `Categorize` / `Bucket` / `Tag` rows. The undo switch learns to
  revert `RuleApply` (same as `Categorize`: restore old value).
- The rule ID is NOT stored in the audit row. The action constant
  tells the operator it was rule-driven; the operator can correlate
  via timestamp + tx ID if they need the rule ID.

## Consequences

- **Bulk-fix is one command.** Operator changes the rule, runs
  `ledger rule apply --overwrite`, all matching transactions are
  updated.
- **Audit log still records everything.** The chain of overwrites is
  in the log: `tx 1 categorized to want (rule X) at T1`,
  `tx 1 categorized to need (rule X --overwrite) at T2`. Replay
  produces current state.
- **Information preservation is partial.** A non-Unknown transaction
  might have been set by hand or by a rule. The `RuleApply` action
  tells you which, but not which rule. (Acceptable trade-off: storing
  `rule_id` would require a new FK on `audit_log`, which is a bigger
  schema change.)
- **Default behavior is unchanged.** Operators who do not pass
  `--overwrite` get the existing no-overwrite semantics. Backward
  compatible.

## Alternatives considered

- **`allow_overwrite` field on the rule itself.** Rejected — adds a
  per-rule declarative flag. The operator's intent is "I'm running
  this rule specifically to fix mistakes," which is better expressed
  as a run-level flag than a rule-level field.
- **New `ledger rule reapply` verb.** Rejected — duplicates
  `rule apply` with a single flag difference. A flag is cleaner.
- **Always overwrite.** Rejected — loses the information-preservation
  story entirely. Operators who want bulk-fix should opt in.
- **Store `rule_id` in the audit log.** Rejected for v1.1 — would
  require a new FK on `audit_log`. The action constant is sufficient
  for distinguishing rule-driven from operator-driven. Revisit if
  per-rule traceability is wanted.

## When this should be revisited

- If the operator wants to trace "which rule changed this tx," add a
  `rule_id` FK to `audit_log`. This is a schema change but adds
  genuine traceability.
- If the operator wants more granular opt-in (per-rule overwrite),
  add `allow_overwrite` to the rule schema.
- If a "rule conflict" emerges — multiple rules with `--overwrite`
  racing on the same transaction — revisit priority semantics. Current
  priority order is by rule priority; with `--overwrite`, the highest-
  priority rule that matches wins.
