package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type AnnotationDeps struct {
	DB         *sql.DB
	TxRepo     ports.TransactionRepository
	TagRepo    ports.TagRepository
	AuditRepo  ports.AuditLogRepository
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