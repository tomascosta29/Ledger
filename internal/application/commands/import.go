package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
	csvinfra "github.com/tomascosta29/Ledger/internal/infrastructure/csv"
)

type ImportDeps struct {
	TxRepo     ports.TransactionRepository
	BatchRepo  ports.ImportBatchRepository
	AuditRepo  ports.AuditLogRepository
	OverlaySvc ports.OverlayService
	Now        func() time.Time
}

type ImportOptions struct {
	File        string
	ProfileName string
	SourceFile  string
	DryRun      bool
}

type ImportStats struct {
	RowsRead     int
	RowsInserted int
	RowsSkipped  int
	BatchID      *int64
}

type ImportResult struct {
	Stats   ImportStats
	Preview []PreviewRow
}

type PreviewRow struct {
	SourceHash    string
	EffectiveDate string
	Amount        string
	Currency      string
	Partner       string
	Description   string
	Duplicate     bool
}

type ImportUseCase struct {
	deps ImportDeps
}

func NewImportUseCase(d ImportDeps) *ImportUseCase {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &ImportUseCase{deps: d}
}

func (u *ImportUseCase) Execute(ctx context.Context, opts ImportOptions) (*ImportResult, error) {
	profile, err := csvinfra.GetProfile(opts.ProfileName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(opts.File)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", opts.File, err)
	}
	defer f.Close()

	rows, err := csvinfra.ParseFile(profile, f, csvinfra.ParseOptions{})
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	result := &ImportResult{
		Stats:   ImportStats{RowsRead: len(rows)},
		Preview: make([]PreviewRow, 0, len(rows)),
	}

	now := u.deps.Now()
	batchID, err := u.deps.BatchRepo.Create(ctx, &entities.ImportBatch{
		SourceFile:    opts.SourceFile,
		SourceProfile: profile.Name,
		RowCount:      len(rows),
		CreatedAt:     now,
	})
	if err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}
	result.Stats.BatchID = &batchID

	if opts.DryRun {
		for _, r := range rows {
			hash := computeHash(profile, r)
			existing, _ := lookupExisting(ctx, u.deps.TxRepo, hash)
			dup := existing != nil
			result.Preview = append(result.Preview, PreviewRow{
				SourceHash:    hash,
				EffectiveDate: r.EffectiveDate,
				Amount:        formatMinor(r.AmountMinor),
				Currency:      string(r.Currency),
				Partner:       r.PartnerName,
				Description:   r.Description,
				Duplicate:     dup,
			})
			if dup {
				result.Stats.RowsSkipped++
			}
		}
		if err := u.deps.BatchRepo.UpdateCounts(ctx, batchID, 0, result.Stats.RowsSkipped); err != nil {
			return nil, fmt.Errorf("update batch counts: %w", err)
		}
		return result, nil
	}

	inserted := 0
	skipped := 0
	auditEntries := make([]*entities.AuditEntry, 0, len(rows))

	for _, r := range rows {
		hash := computeHash(profile, r)
		existing, err := lookupExisting(ctx, u.deps.TxRepo, hash)
		if err != nil {
			return nil, fmt.Errorf("dedup lookup: %w", err)
		}
		if existing != nil {
			skipped++
			continue
		}

		amount, err := valueobjects.New(r.AmountMinor, r.Currency)
		if err != nil {
			return nil, fmt.Errorf("amount: %w", err)
		}
		tx := &entities.Transaction{
			EffectiveDate:  r.EffectiveDate,
			Amount:         amount,
			Description:    r.Description,
			PartnerName:    nullableStr(r.PartnerName),
			PartnerIBAN:    nullableStr(r.PartnerIBAN),
			ImportBatchID:  &batchID,
			SourceHash:     hash,
			RawDescription: nullableStr(r.RawDescription),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		ids, err := u.deps.TxRepo.InsertBatch(ctx, []*entities.Transaction{tx})
		if err != nil {
			return nil, fmt.Errorf("insert tx: %w", err)
		}
		if len(ids) > 0 {
			inserted++
			newID := ids[0]
			auditEntries = append(auditEntries, &entities.AuditEntry{
				TableName: "transactions",
				RecordID:  newID,
				Action:    entities.AuditActionImport,
				CreatedAt: now,
			})
		}
	}

	if err := u.deps.AuditRepo.AppendBatch(ctx, auditEntries); err != nil {
		return nil, fmt.Errorf("append audit: %w", err)
	}

	if err := u.deps.BatchRepo.UpdateCounts(ctx, batchID, inserted, skipped); err != nil {
		return nil, fmt.Errorf("update batch counts: %w", err)
	}

	if u.deps.OverlaySvc != nil {
		if err := u.deps.OverlaySvc.Rebuild(ctx); err != nil {
			return nil, fmt.Errorf("rebuild overlay: %w", err)
		}
	}

	result.Stats.RowsInserted = inserted
	result.Stats.RowsSkipped = skipped
	return result, nil
}

func computeHash(p *csvinfra.Profile, r csvinfra.ParsedRow) string {
	return csvinfra.ComputeSourceHash(csvinfra.HashInput{
		ProfileName:    p.Name,
		ProfileVersion: p.Version,
		BookingDate:    r.EffectiveDate,
		AmountMinor:    r.AmountMinor,
		Currency:       r.Currency,
		PartnerName:    r.PartnerName,
		Description:    r.Description,
	})
}

func lookupExisting(ctx context.Context, repo ports.TransactionRepository, hash string) (*entities.Transaction, error) {
	tx, err := repo.GetBySourceHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return tx, nil
}

func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

func formatMinor(minor int64) string {
	if minor == 0 {
		return "0.00"
	}
	abs := minor
	if abs < 0 {
		abs = -abs
	}
	sign := ""
	if minor < 0 {
		sign = "-"
	}
	whole := abs / 100
	frac := abs % 100
	return fmt.Sprintf("%s%d.%02d", sign, whole, frac)
}
