package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type AnnotationDeps struct {
	DB         *sql.DB
	TxRepo     ports.TransactionRepository
	TagRepo    ports.TagRepository
	AuditRepo  ports.AuditLogRepository
	BatchRepo  ports.ImportBatchRepository
	OverlaySvc ports.OverlayService
	Now        func() time.Time
}

type AnnotationService struct {
	deps AnnotationDeps
}

func NewAnnotationService(d AnnotationDeps) *AnnotationService {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &AnnotationService{deps: d}
}

func (s *AnnotationService) Categorize(ctx context.Context, txID int64, category string) error {
	if category == "" {
		return errors.New("category is empty")
	}
	old, err := s.deps.TxRepo.GetByID(ctx, txID)
	if err != nil {
		return fmt.Errorf("load transaction: %w", err)
	}
	oldCategory := old.Category

	return s.runTx(ctx, func(tx *sql.Tx) error {
		if err := s.deps.TxRepo.SetCategoryDBTX(ctx, tx, txID, category); err != nil {
			return err
		}
		entry := &entities.AuditEntry{
			TableName: "transactions",
			RecordID:  txID,
			Action:    entities.AuditActionCategorize,
			Field:     strPtr("category"),
			OldValue:  strPtr(oldCategory),
			NewValue:  strPtr(category),
			CreatedAt: s.deps.Now(),
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, entry); err != nil {
			return err
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) SetHidden(ctx context.Context, txID int64, hidden bool) error {
	old, err := s.deps.TxRepo.GetByID(ctx, txID)
	if err != nil {
		return fmt.Errorf("load transaction: %w", err)
	}
	if old.IsHidden == hidden {
		return nil
	}

	return s.runTx(ctx, func(tx *sql.Tx) error {
		if err := s.deps.TxRepo.SetHiddenDBTX(ctx, tx, txID, hidden); err != nil {
			return err
		}
		entry := &entities.AuditEntry{
			TableName: "transactions",
			RecordID:  txID,
			Action:    entities.AuditActionVisibility,
			Field:     strPtr("is_hidden"),
			OldValue:  boolStrPtr(old.IsHidden),
			NewValue:  boolStrPtr(hidden),
			CreatedAt: s.deps.Now(),
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, entry); err != nil {
			return err
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) AddTag(ctx context.Context, txID int64, tag string) error {
	existing, err := s.deps.TagRepo.ListByTransaction(ctx, txID)
	if err != nil {
		return fmt.Errorf("load tags: %w", err)
	}
	if contains(existing, tag) {
		return nil
	}

	return s.runTx(ctx, func(tx *sql.Tx) error {
		if err := s.deps.TagRepo.AddDBTX(ctx, tx, txID, tag); err != nil {
			return err
		}
		newTags := append(existing, tag)
		entry := &entities.AuditEntry{
			TableName: "transactions",
			RecordID:  txID,
			Action:    entities.AuditActionTag,
			Field:     strPtr("tags"),
			OldValue:  strPtr(joinTags(existing)),
			NewValue:  strPtr(joinTags(newTags)),
			CreatedAt: s.deps.Now(),
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, entry); err != nil {
			return err
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) RemoveTag(ctx context.Context, txID int64, tag string) error {
	existing, err := s.deps.TagRepo.ListByTransaction(ctx, txID)
	if err != nil {
		return fmt.Errorf("load tags: %w", err)
	}
	if !contains(existing, tag) {
		return nil
	}

	return s.runTx(ctx, func(tx *sql.Tx) error {
		if err := s.deps.TagRepo.RemoveDBTX(ctx, tx, txID, tag); err != nil {
			return err
		}
		remaining := removeFrom(existing, tag)
		entry := &entities.AuditEntry{
			TableName: "transactions",
			RecordID:  txID,
			Action:    entities.AuditActionTag,
			Field:     strPtr("tags"),
			OldValue:  strPtr(joinTags(existing)),
			NewValue:  strPtr(joinTags(remaining)),
			CreatedAt: s.deps.Now(),
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, entry); err != nil {
			return err
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) runTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.deps.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func strPtr(s string) *string { return &s }

func boolStrPtr(b bool) *string {
	if b {
		s := "true"
		return &s
	}
	s := "false"
	return &s
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func removeFrom(s []string, v string) []string {
	out := make([]string, 0, len(s))
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func joinTags(s []string) string {
	if len(s) == 0 {
		return ""
	}
	out := s[0]
	for _, x := range s[1:] {
		out += "," + x
	}
	return out
}

func (s *AnnotationService) Undo(ctx context.Context) error {
	return s.runTx(ctx, func(tx *sql.Tx) error {
		ts, err := s.deps.AuditRepo.FindLatestUndoneTimestampDBTX(ctx, tx)
		if err != nil {
			return fmt.Errorf("find latest undone timestamp: %w", err)
		}
		if ts == "" {
			return errors.New("nothing to undo")
		}

		entries, err := s.deps.AuditRepo.GetByTimestampDBTX(ctx, tx, ts)
		if err != nil {
			return fmt.Errorf("load audit entries: %w", err)
		}
		if len(entries) == 0 {
			return errors.New("nothing to undo")
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM overlay_transactions`); err != nil {
			return fmt.Errorf("clear overlay: %w", err)
		}

		importBatchesToDelete := make(map[int64]bool)

		for _, entry := range entries {
			switch entry.Action {
			case entities.AuditActionImport:
				txn, err := s.deps.TxRepo.GetByIDDBTX(ctx, tx, entry.RecordID)
				if err != nil {
					if errors.Is(err, ports.ErrNotFound) {
						continue
					}
					return fmt.Errorf("get transaction %d: %w", entry.RecordID, err)
				}
				if txn.ImportBatchID != nil {
					importBatchesToDelete[*txn.ImportBatchID] = true
				}
				if err := s.deps.TxRepo.DeleteDBTX(ctx, tx, entry.RecordID); err != nil {
					return fmt.Errorf("delete transaction %d: %w", entry.RecordID, err)
				}

			case entities.AuditActionCategorize:
				category := "Unknown"
				if entry.OldValue != nil {
					category = *entry.OldValue
				}
				if err := s.deps.TxRepo.SetCategoryDBTX(ctx, tx, entry.RecordID, category); err != nil {
					return fmt.Errorf("restore category for txn %d: %w", entry.RecordID, err)
				}

			case entities.AuditActionVisibility:
				hidden := false
				if entry.OldValue != nil && *entry.OldValue == "true" {
					hidden = true
				}
				if err := s.deps.TxRepo.SetHiddenDBTX(ctx, tx, entry.RecordID, hidden); err != nil {
					return fmt.Errorf("restore hidden for txn %d: %w", entry.RecordID, err)
				}

			case entities.AuditActionTag:
				if err := s.deps.TagRepo.ClearDBTX(ctx, tx, entry.RecordID); err != nil {
					return fmt.Errorf("clear tags for txn %d: %w", entry.RecordID, err)
				}
				if entry.OldValue != nil && *entry.OldValue != "" {
					tags := strings.Split(*entry.OldValue, ",")
					for _, tag := range tags {
						tag = strings.TrimSpace(tag)
						if tag != "" {
							if err := s.deps.TagRepo.AddDBTX(ctx, tx, entry.RecordID, tag); err != nil {
								return fmt.Errorf("restore tag %q for txn %d: %w", tag, entry.RecordID, err)
							}
						}
					}
				}

			default:
				return fmt.Errorf("cannot undo action %q: unsupported action type", entry.Action)
			}
		}

		for batchID := range importBatchesToDelete {
			if s.deps.BatchRepo != nil {
				if err := s.deps.BatchRepo.DeleteDBTX(ctx, tx, batchID); err != nil {
					return fmt.Errorf("delete empty import batch %d: %w", batchID, err)
				}
			}
		}

		undoEntry := &entities.AuditEntry{
			TableName: "audit_log",
			RecordID:  0,
			Action:    entities.AuditActionUndo,
			OldValue:  &ts,
			CreatedAt: s.deps.Now(),
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, undoEntry); err != nil {
			return fmt.Errorf("append undo audit entry: %w", err)
		}

		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}
