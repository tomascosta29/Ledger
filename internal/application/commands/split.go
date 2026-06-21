package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
	csvinfra "github.com/tomascosta29/Ledger/internal/infrastructure/csv"
)

type SplitDeps struct {
	TxRepo     ports.TransactionRepository
	AuditRepo  ports.AuditLogRepository
	OverlaySvc ports.OverlayService
	Now        func() time.Time
}

type SplitChild struct {
	AmountMinor int64
	Currency    valueobjects.Currency
	Description string
}

type SplitOptions struct {
	TransactionID int64
	Children      []SplitChild
}

type SplitResult struct {
	ParentID    int64
	ChildrenIDs []int64
}

type SplitUseCase struct {
	deps SplitDeps
}

func NewSplitUseCase(d SplitDeps) *SplitUseCase {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &SplitUseCase{deps: d}
}

func (u *SplitUseCase) Execute(ctx context.Context, opts SplitOptions) (*SplitResult, error) {
	if len(opts.Children) < 2 {
		return nil, errors.New("split requires at least 2 children")
	}

	parent, err := u.deps.TxRepo.GetByID(ctx, opts.TransactionID)
	if err != nil {
		return nil, fmt.Errorf("load parent: %w", err)
	}
	if parent.ParentTxnID != nil {
		return nil, fmt.Errorf("transaction %d is itself a split child; cannot re-split", parent.ID)
	}
	// Check if the parent already has children.
	existing, err := u.deps.TxRepo.FindByParent(ctx, parent.ID)
	if err != nil {
		return nil, fmt.Errorf("check children: %w", err)
	}
	if len(existing) > 0 {
		return nil, fmt.Errorf("transaction %d already has %d child(ren); undo the split first", parent.ID, len(existing))
	}

	var sum int64
	for i, c := range opts.Children {
		if c.Currency != parent.Amount.Currency {
			return nil, fmt.Errorf("child %d currency %s does not match parent %s", i+1, c.Currency, parent.Amount.Currency)
		}
		if c.AmountMinor == 0 {
			return nil, fmt.Errorf("child %d has zero amount", i+1)
		}
		sum += c.AmountMinor
	}
	if sum != parent.Amount.Amount {
		return nil, fmt.Errorf("children sum %d does not equal parent %d", sum, parent.Amount.Amount)
	}

	now := u.deps.Now()
	children := make([]*entities.Transaction, 0, len(opts.Children))
	for _, c := range opts.Children {
		hash := csvinfra.ComputeSourceHash(csvinfra.HashInput{
			ProfileName:    "split",
			ProfileVersion: 1,
			BookingDate:    parent.EffectiveDate,
			AmountMinor:    c.AmountMinor,
			Currency:       c.Currency,
			PartnerName:    derefStr(parent.PartnerName),
			Description:    fmt.Sprintf("%s|child:%s", parent.SourceHash, strings.TrimSpace(c.Description)),
		})
		children = append(children, &entities.Transaction{
			EffectiveDate:  parent.EffectiveDate,
			Amount:         valueobjects.MustNew(c.AmountMinor, c.Currency),
			Description:    c.Description,
			PartnerName:    parent.PartnerName,
			PartnerIBAN:    parent.PartnerIBAN,
			ParentTxnID:    &parent.ID,
			ImportBatchID:  parent.ImportBatchID,
			SourceHash:     hash,
			CategoryID:     parent.CategoryID,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
	}

	ids, err := u.deps.TxRepo.InsertBatch(ctx, children)
	if err != nil {
		return nil, fmt.Errorf("insert children: %w", err)
	}

	// Single audit row on the parent recording the split. NewValue is the
	// comma-separated child IDs so Undo can find and delete them.
	childIDStrs := make([]string, len(ids))
	for i, id := range ids {
		childIDStrs[i] = fmt.Sprintf("%d", id)
	}
	auditEntries := []*entities.AuditEntry{{
		TableName: "transactions",
		RecordID:  parent.ID,
		Action:    entities.AuditActionSplit,
		Field:     stringPtr("children"),
		OldValue:  nil,
		NewValue:  stringPtr(strings.Join(childIDStrs, ",")),
		CreatedAt: now,
	}}
	if err := u.deps.AuditRepo.AppendBatch(ctx, auditEntries); err != nil {
		return nil, fmt.Errorf("append audit: %w", err)
	}

	if u.deps.OverlaySvc != nil {
		if err := u.deps.OverlaySvc.Rebuild(ctx); err != nil {
			return nil, fmt.Errorf("rebuild overlay: %w", err)
		}
	}

	return &SplitResult{ParentID: parent.ID, ChildrenIDs: ids}, nil
}

func stringPtr(s string) *string { return &s }

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
