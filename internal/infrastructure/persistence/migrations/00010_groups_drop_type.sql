-- +goose Up
-- Per ADR 0006: the `type` column on transaction_groups is redundant
-- with the audit log. The action constants (transfer_linked vs
-- reimbursement_linked) preserve the historical distinction, and
-- current-state queries resolve the type from the partner data.
ALTER TABLE transaction_groups DROP COLUMN type;

-- +goose Down
-- Recreate the column for a fresh downgrade. Default to 'transfer'
-- since that's the most common case in the operator's data.
ALTER TABLE transaction_groups ADD COLUMN type TEXT NOT NULL DEFAULT 'transfer';
