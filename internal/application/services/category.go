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

type CategoryDeps struct {
	DB         *sql.DB
	CategoryRepo ports.CategoryRepository
	AuditRepo  ports.AuditLogRepository
	Now        func() time.Time
}

type CategoryService struct {
	deps CategoryDeps
}

func NewCategoryService(d CategoryDeps) *CategoryService {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &CategoryService{deps: d}
}

// Create inserts a new category and writes one category_create audit row,
// all in a single tx.
func (s *CategoryService) Create(ctx context.Context, name, description string) (*entities.Category, error) {
	if name == "" {
		return nil, errors.New("category name is empty")
	}
	if existing, err := s.deps.CategoryRepo.GetByName(ctx, name); err == nil && existing != nil {
		return nil, fmt.Errorf("category %q already exists", name)
	} else if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return nil, fmt.Errorf("lookup category: %w", err)
	}

	var created *entities.Category
	err := s.runTx(ctx, func(tx *sql.Tx) error {
		now := s.deps.Now()
		c := &entities.Category{Name: name, Description: description, CreatedAt: now}
		id, err := s.deps.CategoryRepo.CreateDBTX(ctx, tx, c)
		if err != nil {
			return err
		}
		c.ID = id
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, &entities.AuditEntry{
			TableName: "categories",
			RecordID:  id,
			Action:    entities.AuditActionCategoryCreate,
			Field:     strPtr("name"),
			OldValue:  nil,
			NewValue:  strPtr(name),
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("append audit: %w", err)
		}
		created = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Rename changes a category's name and writes one category_rename audit row.
func (s *CategoryService) Rename(ctx context.Context, oldName, newName string) error {
	if newName == "" {
		return errors.New("new category name is empty")
	}
	old, err := s.deps.CategoryRepo.GetByName(ctx, oldName)
	if err != nil {
		return fmt.Errorf("lookup old name: %w", err)
	}
	if existing, err := s.deps.CategoryRepo.GetByName(ctx, newName); err == nil && existing != nil && existing.ID != old.ID {
		return fmt.Errorf("category %q already exists", newName)
	} else if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("lookup new name: %w", err)
	}

	return s.runTx(ctx, func(tx *sql.Tx) error {
		now := s.deps.Now()
		if err := s.deps.CategoryRepo.RenameDBTX(ctx, tx, old.ID, newName); err != nil {
			return err
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, &entities.AuditEntry{
			TableName: "categories",
			RecordID:  old.ID,
			Action:    entities.AuditActionCategoryRename,
			Field:     strPtr("name"),
			OldValue:  strPtr(oldName),
			NewValue:  strPtr(newName),
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("append audit: %w", err)
		}
		return nil
	})
}

// Archive sets archived_at and writes one category_archive audit row.
func (s *CategoryService) Archive(ctx context.Context, name string) error {
	c, err := s.deps.CategoryRepo.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("lookup category: %w", err)
	}
	if c.ArchivedAt != nil {
		return fmt.Errorf("category %q is already archived", name)
	}
	return s.runTx(ctx, func(tx *sql.Tx) error {
		now := s.deps.Now()
		if err := s.deps.CategoryRepo.ArchiveDBTX(ctx, tx, c.ID); err != nil {
			return err
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, &entities.AuditEntry{
			TableName: "categories",
			RecordID:  c.ID,
			Action:    entities.AuditActionCategoryArchive,
			Field:     strPtr("archived_at"),
			OldValue:  nil,
			NewValue:  strPtr(now.Format("2006-01-02T15:04:05.000Z")),
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("append audit: %w", err)
		}
		return nil
	})
}

func (s *CategoryService) runTx(ctx context.Context, fn func(*sql.Tx) error) error {
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
