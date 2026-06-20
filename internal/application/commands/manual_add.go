package commands

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
	csvinfra "github.com/tomascosta29/Ledger/internal/infrastructure/csv"
)

type ManualAddDeps struct {
	TxRepo     ports.TransactionRepository
	AuditRepo  ports.AuditLogRepository
	OverlaySvc ports.OverlayService
	Now        func() time.Time
}

type ManualAddOptions struct {
	EffectiveDate string
	Amount        string
	Currency      string
	Description   string
	PartnerName   string
	PartnerIBAN   string
	Category      string
	BucketName    string
}

type ManualAddResult struct {
	TransactionID int64
	Created       bool
}

type ManualAddUseCase struct {
	deps ManualAddDeps
}

func NewManualAddUseCase(d ManualAddDeps) *ManualAddUseCase {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &ManualAddUseCase{deps: d}
}

func (u *ManualAddUseCase) Execute(ctx context.Context, opts ManualAddOptions) (*ManualAddResult, error) {
	if opts.EffectiveDate == "" {
		return nil, errors.New("--date is required (YYYY-MM-DD)")
	}
	if opts.Currency == "" {
		return nil, errors.New("--currency is required (e.g. EUR)")
	}
	if opts.Description == "" {
		return nil, errors.New("--description is required")
	}

	cur := valueobjects.Currency(opts.Currency)
	money, err := valueobjects.ParseDecimal(opts.Amount, cur)
	if err != nil {
		return nil, fmt.Errorf("amount: %w", err)
	}

	category := opts.Category
	if category == "" {
		category = "Unknown"
	}

	hash := csvinfra.ComputeSourceHash(csvinfra.HashInput{
		ProfileName:    "manual",
		ProfileVersion: 1,
		BookingDate:    opts.EffectiveDate,
		AmountMinor:    money.Amount,
		Currency:       cur,
		PartnerName:    opts.PartnerName,
		Description:    opts.Description,
	})

	now := u.deps.Now()
	tx := &entities.Transaction{
		EffectiveDate:  opts.EffectiveDate,
		Amount:         money,
		Description:    opts.Description,
		PartnerName:    nullableStr(opts.PartnerName),
		PartnerIBAN:    nullableStr(opts.PartnerIBAN),
		SourceHash:     hash,
		RawDescription: nullableStr(opts.Description),
		Category:       category,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	existing, err := lookupExisting(ctx, u.deps.TxRepo, hash)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &ManualAddResult{TransactionID: existing.ID, Created: false}, nil
	}

	id, err := u.deps.TxRepo.Insert(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("insert: %w", err)
	}

	if _, err := u.deps.AuditRepo.Append(ctx, &entities.AuditEntry{
		TableName: "transactions",
		RecordID:  id,
		Action:    entities.AuditActionImport,
		CreatedAt: now,
	}); err != nil {
		return nil, fmt.Errorf("append audit: %w", err)
	}

	if u.deps.OverlaySvc != nil {
		if err := u.deps.OverlaySvc.Rebuild(ctx); err != nil {
			return nil, fmt.Errorf("rebuild overlay: %w", err)
		}
	}

	return &ManualAddResult{TransactionID: id, Created: true}, nil
}
