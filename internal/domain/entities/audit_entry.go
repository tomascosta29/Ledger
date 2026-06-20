package entities

import "time"

type AuditEntry struct {
	ID        int64
	TableName string
	RecordID  int64
	Action    string
	Field     *string
	OldValue  *string
	NewValue  *string
	CreatedAt time.Time
}

const (
	AuditActionImport            = "import"
	AuditActionEdit              = "edit"
	AuditActionVisibility        = "visibility"
	AuditActionCategorize        = "categorize"
	AuditActionTag               = "tag"
	AuditActionBucket            = "bucket_assign"
	AuditActionSplit             = "split"
	AuditActionTransferLink      = "transfer_linked"
	AuditActionReimbursementLink = "reimbursement_linked"
	AuditActionUndo              = "undo"
	AuditActionBatch             = "batch"
)
