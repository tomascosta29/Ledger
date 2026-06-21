package services

import (
	"context"
	"fmt"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type ReimbursementDeps struct {
	TxRepo    ports.TransactionRepository
	GroupRepo ports.GroupRepository
	AuditRepo ports.AuditLogRepository
	OverlaySvc ports.OverlayService
	Now       func() time.Time
}

type ReimbursementService struct {
	deps ReimbursementDeps
}

func NewReimbursementService(d ReimbursementDeps) *ReimbursementService {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &ReimbursementService{deps: d}
}

func (s *ReimbursementService) Link(ctx context.Context, txIDs []int64) (int64, error) {
	if len(txIDs) < 2 {
		return 0, fmt.Errorf("need at least 2 transactions to form a reimbursement group")
	}
	if len(txIDs) > 2 {
		return 0, fmt.Errorf("v1 only supports 2-member reimbursement groups (got %d)", len(txIDs))
	}
	a, err := s.deps.TxRepo.GetByID(ctx, txIDs[0])
	if err != nil {
		return 0, fmt.Errorf("load %d: %w", txIDs[0], err)
	}
	b, err := s.deps.TxRepo.GetByID(ctx, txIDs[1])
	if err != nil {
		return 0, fmt.Errorf("load %d: %w", txIDs[1], err)
	}
	if a.Amount.Currency != b.Amount.Currency {
		return 0, fmt.Errorf("currency mismatch: %s vs %s", a.Amount.Currency, b.Amount.Currency)
	}
	if a.Amount.Sign() == b.Amount.Sign() {
		return 0, fmt.Errorf("both transactions have the same sign; expected expense + reimbursement")
	}

	groupID, err := s.deps.GroupRepo.CreateGroup(ctx, &entities.TransactionGroup{})
	if err != nil {
		return 0, err
	}
	if err := s.deps.GroupRepo.AddMember(ctx, groupID, a.ID, "expense"); err != nil {
		return 0, err
	}
	if err := s.deps.GroupRepo.AddMember(ctx, groupID, b.ID, "reimbursement"); err != nil {
		return 0, err
	}

	now := s.deps.Now()
	entries := []*entities.AuditEntry{
		{TableName: "transactions", RecordID: a.ID, Action: entities.AuditActionReimbursementLink,
			Field: linkStr("group_id"), NewValue: i64Str(groupID), CreatedAt: now},
		{TableName: "transactions", RecordID: b.ID, Action: entities.AuditActionReimbursementLink,
			Field: linkStr("group_id"), NewValue: i64Str(groupID), CreatedAt: now},
	}
	if err := s.deps.AuditRepo.AppendBatch(ctx, entries); err != nil {
		return 0, fmt.Errorf("append audit: %w", err)
	}
	if s.deps.OverlaySvc != nil {
		if err := s.deps.OverlaySvc.Rebuild(ctx); err != nil {
			return 0, fmt.Errorf("rebuild overlay: %w", err)
		}
	}
	return groupID, nil
}

func linkStr(s string) *string { return &s }
func i64Str(i int64) *string    { s := fmt.Sprintf("%d", i); return &s }
