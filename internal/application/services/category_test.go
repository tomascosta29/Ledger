package services_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

type categoryTestEnv struct {
	db       *persistence.DB
	catSvc   *services.CategoryService
	catRepo  *persistence.CategoryRepository
	auditRepo *persistence.AuditLogRepository
	annSvc   *services.AnnotationService
	cleanup  func()
}

func newCategoryTestEnv(t *testing.T) *categoryTestEnv {
	t.Helper()
	db, err := persistence.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	catRepo := persistence.NewCategoryRepository(db)
	auditRepo := persistence.NewAuditLogRepository(db)
	txRepo := persistence.NewTransactionRepository(db)
	tagRepo := persistence.NewTagRepository(db)
	bucketRepo := persistence.NewBucketRepository(db)
	batchRepo := persistence.NewImportBatchRepository(db)
	ovSvc := services.NewOverlayService(db.DB)

	catSvc := services.NewCategoryService(services.CategoryDeps{
		DB:          db.DB,
		CategoryRepo: catRepo,
		AuditRepo:   auditRepo,
	})
	annSvc := services.NewAnnotationService(services.AnnotationDeps{
		DB:           db.DB,
		TxRepo:       txRepo,
		TagRepo:      tagRepo,
		BucketRepo:   bucketRepo,
		CategoryRepo: catRepo,
		AuditRepo:    auditRepo,
		BatchRepo:    batchRepo,
		OverlaySvc:   ovSvc,
	})
	return &categoryTestEnv{
		db:        db,
		catSvc:    catSvc,
		catRepo:   catRepo,
		auditRepo: auditRepo,
		annSvc:    annSvc,
		cleanup:   func() { _ = db.Close() },
	}
}

func TestCategoryCreateWritesAuditRow(t *testing.T) {
	env := newCategoryTestEnv(t)
	defer env.cleanup()

	c, err := env.catSvc.Create(context.Background(), "want", "discretionary")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ID == 0 || c.Name != "want" {
		t.Fatalf("unexpected result: %+v", c)
	}

	entries, err := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{
		TableName: strPtr("categories"),
		RecordID:  &c.ID,
	})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(entries))
	}
	if entries[0].Action != entities.AuditActionCategoryCreate {
		t.Fatalf("action = %q, want %q", entries[0].Action, entities.AuditActionCategoryCreate)
	}
	if entries[0].NewValue == nil || *entries[0].NewValue != "want" {
		t.Fatalf("new value = %v, want \"want\"", entries[0].NewValue)
	}
}

func TestCategoryCreateRejectsDuplicate(t *testing.T) {
	env := newCategoryTestEnv(t)
	defer env.cleanup()

	if _, err := env.catSvc.Create(context.Background(), "want", ""); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := env.catSvc.Create(context.Background(), "want", "")
	if err == nil {
		t.Fatal("expected error on duplicate name")
	}
	if !errors.Is(err, ports.ErrNotFound) && !strContains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCategoryRenameWritesAuditRow(t *testing.T) {
	env := newCategoryTestEnv(t)
	defer env.cleanup()

	if _, err := env.catSvc.Create(context.Background(), "want", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := env.catSvc.Rename(context.Background(), "want", "discretionary"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got, err := env.catRepo.GetByName(context.Background(), "discretionary")
	if err != nil {
		t.Fatalf("lookup new name: %v", err)
	}
	if got.Name != "discretionary" {
		t.Fatalf("name = %q", got.Name)
	}

	entries, err := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{
		Action: strPtr(entities.AuditActionCategoryRename),
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 rename row, got %d", len(entries))
	}
	if entries[0].OldValue == nil || *entries[0].OldValue != "want" {
		t.Fatalf("old = %v, want \"want\"", entries[0].OldValue)
	}
	if entries[0].NewValue == nil || *entries[0].NewValue != "discretionary" {
		t.Fatalf("new = %v, want \"discretionary\"", entries[0].NewValue)
	}
}

func TestCategoryArchiveWritesAuditRow(t *testing.T) {
	env := newCategoryTestEnv(t)
	defer env.cleanup()

	if _, err := env.catSvc.Create(context.Background(), "want", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := env.catSvc.Archive(context.Background(), "want"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	c, err := env.catRepo.GetByName(context.Background(), "want")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if c.ArchivedAt == nil {
		t.Fatal("expected archived_at to be set")
	}

	entries, _ := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{
		Action: strPtr(entities.AuditActionCategoryArchive),
	})
	if len(entries) != 1 {
		t.Fatalf("expected 1 archive row, got %d", len(entries))
	}
}

func TestUndoArchiveUnarchives(t *testing.T) {
	env := newCategoryTestEnv(t)
	defer env.cleanup()

	if _, err := env.catSvc.Create(context.Background(), "want", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := env.catSvc.Archive(context.Background(), "want"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := env.annSvc.Undo(context.Background()); err != nil {
		t.Fatalf("undo: %v", err)
	}

	c, err := env.catRepo.GetByName(context.Background(), "want")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if c.ArchivedAt != nil {
		t.Fatalf("expected archived_at=nil after undo, got %v", *c.ArchivedAt)
	}
}

func TestUndoRenameRestoresName(t *testing.T) {
	env := newCategoryTestEnv(t)
	defer env.cleanup()

	if _, err := env.catSvc.Create(context.Background(), "want", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := env.catSvc.Rename(context.Background(), "want", "discretionary"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := env.annSvc.Undo(context.Background()); err != nil {
		t.Fatalf("undo: %v", err)
	}

	c, err := env.catRepo.GetByName(context.Background(), "want")
	if err != nil {
		t.Fatalf("lookup original name: %v", err)
	}
	if c.Name != "want" {
		t.Fatalf("expected name \"want\" after undo, got %q", c.Name)
	}
	if _, err := env.catRepo.GetByName(context.Background(), "discretionary"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("expected old name not found, got err=%v", err)
	}
}

func strContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
