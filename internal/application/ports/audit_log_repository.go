package ports

import (
	"context"

	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type AuditEntryFilter struct {
	TableName *string
	RecordID  *int64
	Action    *string
	Since     *string
	Limit     int
}

type AuditLogRepository interface {
	Append(ctx context.Context, entry *entities.AuditEntry) (int64, error)
	AppendBatch(ctx context.Context, entries []*entities.AuditEntry) error
	Query(ctx context.Context, filter AuditEntryFilter) ([]*entities.AuditEntry, error)
	LastBatch(ctx context.Context, tableName string, recordID int64) ([]*entities.AuditEntry, error)
}