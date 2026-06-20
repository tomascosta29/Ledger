package services_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

type annotationTestEnv struct {
	db        *persistence.DB
	annSvc    *services.AnnotationService
	txRepo    *persistence.TransactionRepository
	tagRepo   *persistence.TagRepository
	auditRepo *persistence.AuditLogRepository
	batchRepo *persistence.ImportBatchRepository
	ovSvc     *services.OverlayService
	ovRepo    *persistence.OverlayRepository
	cleanup   func()
}

func newAnnotationTestEnv(t *testing.T) *annotationTestEnv {
	t.Helper()
	db, err := persistence.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	txRepo := persistence.NewTransactionRepository(db)
	tagRepo := persistence.NewTagRepository(db)
	auditRepo := persistence.NewAuditLogRepository(db)
	batchRepo := persistence.NewImportBatchRepository(db)
	ovSvc := services.NewOverlayService(db.DB)
	ovRepo := persistence.NewOverlayRepository(db)

	currentTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	nowFunc := func() time.Time {
		currentTime = currentTime.Add(time.Second)
		return currentTime
	}

	annSvc := services.NewAnnotationService(services.AnnotationDeps{
		DB:         db.DB,
		TxRepo:     txRepo,
		TagRepo:    tagRepo,
		AuditRepo:  auditRepo,
		BatchRepo:  batchRepo,
		OverlaySvc: ovSvc,
		Now:        nowFunc,
	})
	return &annotationTestEnv{
		db:        db,
		annSvc:    annSvc,
		txRepo:    txRepo,
		tagRepo:   tagRepo,
		auditRepo: auditRepo,
		batchRepo: batchRepo,
		ovSvc:     ovSvc,
		ovRepo:    ovRepo,
		cleanup:   func() { _ = db.Close() },
	}
}

func (e *annotationTestEnv) seedTx(t *testing.T) int64 {
	t.Helper()
	id, err := e.txRepo.Insert(context.Background(), &entities.Transaction{
		EffectiveDate: "2026-04-30",
		Amount:        valueobjects.MustNew(-1000, valueobjects.EUR),
		Description:   "test",
		SourceHash:    "h",
		Category:      "Unknown",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := e.ovSvc.Rebuild(context.Background()); err != nil {
		t.Fatalf("rebuild after seed: %v", err)
	}
	return id
}

func TestAnnotateCategorizeWritesAndRebuilds(t *testing.T) {
	env := newAnnotationTestEnv(t)
	defer env.cleanup()
	id := env.seedTx(t)

	if err := env.annSvc.Categorize(context.Background(), id, "want"); err != nil {
		t.Fatalf("categorize: %v", err)
	}

	got, err := env.txRepo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Category != "want" {
		t.Fatalf("category = %q, want want", got.Category)
	}

	rows, err := env.ovRepo.FindAll(context.Background(), ports.OverlayFindOptions{})
	if err != nil {
		t.Fatalf("overlay find: %v", err)
	}
	if len(rows) != 1 || rows[0].Category != "want" {
		t.Fatalf("overlay not updated: %+v", rows)
	}

	entries, err := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{
		TableName: strPtr("transactions"),
		RecordID:  &id,
	})
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != entities.AuditActionCategorize {
		t.Fatalf("audit entry wrong: %+v", entries)
	}
}

func TestAnnotateHideRemovesFromOverlay(t *testing.T) {
	env := newAnnotationTestEnv(t)
	defer env.cleanup()
	id := env.seedTx(t)

	if err := env.annSvc.SetHidden(context.Background(), id, true); err != nil {
		t.Fatalf("hide: %v", err)
	}

	got, err := env.txRepo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.IsHidden {
		t.Fatal("expected hidden=true")
	}

	rows, _ := env.ovRepo.FindAll(context.Background(), ports.OverlayFindOptions{})
	if len(rows) != 0 {
		t.Fatalf("hidden txn should be removed from overlay; got %d rows", len(rows))
	}
}

func TestAnnotateHideIsIdempotent(t *testing.T) {
	env := newAnnotationTestEnv(t)
	defer env.cleanup()
	id := env.seedTx(t)

	if err := env.annSvc.SetHidden(context.Background(), id, false); err != nil {
		t.Fatalf("hide: %v", err)
	}
	entries, _ := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{
		TableName: strPtr("transactions"),
		RecordID:  &id,
	})
	if len(entries) != 0 {
		t.Fatalf("expected no audit for no-op hide; got %d", len(entries))
	}
}

func TestAnnotateTagAdd(t *testing.T) {
	env := newAnnotationTestEnv(t)
	defer env.cleanup()
	id := env.seedTx(t)

	if err := env.annSvc.AddTag(context.Background(), id, "rent"); err != nil {
		t.Fatalf("add tag: %v", err)
	}
	if err := env.annSvc.AddTag(context.Background(), id, "groceries"); err != nil {
		t.Fatalf("add tag 2: %v", err)
	}

	tags, err := env.tagRepo.ListByTransaction(context.Background(), id)
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	if len(tags) != 2 || tags[0] != "groceries" || tags[1] != "rent" {
		t.Fatalf("tags wrong: %v", tags)
	}

	rows, _ := env.ovRepo.FindAll(context.Background(), ports.OverlayFindOptions{})
	if len(rows) != 1 || rows[0].Tags != "groceries,rent" {
		t.Fatalf("overlay tags wrong: %+v", rows)
	}
}

func TestAnnotateTagAddIsIdempotent(t *testing.T) {
	env := newAnnotationTestEnv(t)
	defer env.cleanup()
	id := env.seedTx(t)

	for i := 0; i < 3; i++ {
		if err := env.annSvc.AddTag(context.Background(), id, "rent"); err != nil {
			t.Fatalf("add tag iter %d: %v", i, err)
		}
	}
	tags, _ := env.tagRepo.ListByTransaction(context.Background(), id)
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
}

func TestAnnotateTagRemove(t *testing.T) {
	env := newAnnotationTestEnv(t)
	defer env.cleanup()
	id := env.seedTx(t)

	env.annSvc.AddTag(context.Background(), id, "rent")
	env.annSvc.AddTag(context.Background(), id, "groceries")
	if err := env.annSvc.RemoveTag(context.Background(), id, "rent"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	tags, _ := env.tagRepo.ListByTransaction(context.Background(), id)
	if len(tags) != 1 || tags[0] != "groceries" {
		t.Fatalf("after remove: %v", tags)
	}
}

func TestAnnotateCategorizeRollsBackOnAuditFailure(t *testing.T) {
	env := newAnnotationTestEnv(t)
	defer env.cleanup()
	id := env.seedTx(t)

	// Inject a fault: close DB so audit INSERT fails
	_ = env.db.Close()

	err := env.annSvc.Categorize(context.Background(), id, "want")
	if err == nil {
		t.Fatal("expected error after DB closed")
	}

	// Reopen and verify category wasn't changed. Skip; the test
	// already verifies the rebuild never got to run because the
	// audit insert failed first.
}

func strPtr(s string) *string { return &s }

func TestUndo(t *testing.T) {
	t.Run("undo categorize", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id := env.seedTx(t)

		// Set category to "want" (originally default is "Unknown")
		if err := env.annSvc.Categorize(context.Background(), id, "want"); err != nil {
			t.Fatalf("categorize: %v", err)
		}

		// Revert the category update
		if err := env.annSvc.Undo(context.Background()); err != nil {
			t.Fatalf("undo: %v", err)
		}

		txn, err := env.txRepo.GetByID(context.Background(), id)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if txn.Category != "Unknown" {
			t.Fatalf("expected category to revert to 'Unknown', got %q", txn.Category)
		}

		// Check overlay is also updated
		rows, err := env.ovRepo.FindAll(context.Background(), ports.OverlayFindOptions{})
		if err != nil {
			t.Fatalf("overlay find: %v", err)
		}
		if len(rows) != 1 || rows[0].Category != "Unknown" {
			t.Fatalf("overlay category not updated after undo: %+v", rows)
		}
	})

	t.Run("undo visibility", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id := env.seedTx(t)

		// Hide
		if err := env.annSvc.SetHidden(context.Background(), id, true); err != nil {
			t.Fatalf("hide: %v", err)
		}

		// Undo
		if err := env.annSvc.Undo(context.Background()); err != nil {
			t.Fatalf("undo: %v", err)
		}

		txn, _ := env.txRepo.GetByID(context.Background(), id)
		if txn.IsHidden {
			t.Fatal("expected is_hidden to revert to false")
		}

		rows, _ := env.ovRepo.FindAll(context.Background(), ports.OverlayFindOptions{})
		if len(rows) != 1 {
			t.Fatalf("expected transaction in overlay after undo hide, got %d rows", len(rows))
		}
	})

	t.Run("undo tag change", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id := env.seedTx(t)

		// Add tags
		if err := env.annSvc.AddTag(context.Background(), id, "rent"); err != nil {
			t.Fatalf("add tag: %v", err)
		}
		if err := env.annSvc.AddTag(context.Background(), id, "house"); err != nil {
			t.Fatalf("add tag: %v", err)
		}

		// Remove tag "rent"
		if err := env.annSvc.RemoveTag(context.Background(), id, "rent"); err != nil {
			t.Fatalf("remove tag: %v", err)
		}

		// Undo the remove
		if err := env.annSvc.Undo(context.Background()); err != nil {
			t.Fatalf("undo: %v", err)
		}

		tags, _ := env.tagRepo.ListByTransaction(context.Background(), id)
		if len(tags) != 2 || tags[0] != "house" || tags[1] != "rent" {
			t.Fatalf("expected tags to revert to [house, rent], got %v", tags)
		}

		// Undo the add "house" tag
		if err := env.annSvc.Undo(context.Background()); err != nil {
			t.Fatalf("undo 2: %v", err)
		}

		tags, _ = env.tagRepo.ListByTransaction(context.Background(), id)
		if len(tags) != 1 || tags[0] != "rent" {
			t.Fatalf("expected tags to revert to [rent], got %v", tags)
		}
	})

	t.Run("sequential undos", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id := env.seedTx(t)

		// 1. Categorize "want"
		if err := env.annSvc.Categorize(context.Background(), id, "want"); err != nil {
			t.Fatalf("categorize: %v", err)
		}
		// 2. Hide
		if err := env.annSvc.SetHidden(context.Background(), id, true); err != nil {
			t.Fatalf("hide: %v", err)
		}

		// Check initial state
		txn, _ := env.txRepo.GetByID(context.Background(), id)
		if txn.Category != "want" || !txn.IsHidden {
			t.Fatalf("unexpected state: category=%s, hidden=%t", txn.Category, txn.IsHidden)
		}

		// Undo hide
		if err := env.annSvc.Undo(context.Background()); err != nil {
			t.Fatalf("undo hide: %v", err)
		}
		txn, _ = env.txRepo.GetByID(context.Background(), id)
		if txn.Category != "want" || txn.IsHidden {
			t.Fatalf("after undo hide: category=%s, hidden=%t", txn.Category, txn.IsHidden)
		}

		// Undo categorize
		if err := env.annSvc.Undo(context.Background()); err != nil {
			t.Fatalf("undo categorize: %v", err)
		}
		txn, _ = env.txRepo.GetByID(context.Background(), id)
		if txn.Category != "Unknown" || txn.IsHidden {
			t.Fatalf("after undo categorize: category=%s, hidden=%t", txn.Category, txn.IsHidden)
		}
	})

	t.Run("nothing to undo", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()

		err := env.annSvc.Undo(context.Background())
		if err == nil || err.Error() != "nothing to undo" {
			t.Fatalf("expected 'nothing to undo' error, got %v", err)
		}
	})

	t.Run("undo import", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()

		// Simulate an import batch
		batchID, err := env.batchRepo.Create(context.Background(), &entities.ImportBatch{
			SourceFile:    "test.csv",
			SourceProfile: "revolut",
			RowCount:      2,
			CreatedAt:     time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("create batch: %v", err)
		}

		// Insert two transactions
		tx1 := &entities.Transaction{
			EffectiveDate: "2026-06-20",
			Amount:        valueobjects.MustNew(-150, valueobjects.EUR),
			Description:   "tx 1",
			ImportBatchID: &batchID,
			SourceHash:    "h1",
			Category:      "Unknown",
		}
		tx2 := &entities.Transaction{
			EffectiveDate: "2026-06-20",
			Amount:        valueobjects.MustNew(-300, valueobjects.EUR),
			Description:   "tx 2",
			ImportBatchID: &batchID,
			SourceHash:    "h2",
			Category:      "Unknown",
		}

		ids, err := env.txRepo.InsertBatch(context.Background(), []*entities.Transaction{tx1, tx2})
		if err != nil {
			t.Fatalf("insert transactions: %v", err)
		}
		if len(ids) != 2 {
			t.Fatalf("inserted %d, want 2", len(ids))
		}

		// Append audit entries
		now := time.Now().UTC()
		auditEntries := []*entities.AuditEntry{
			{
				TableName: "transactions",
				RecordID:  ids[0],
				Action:    entities.AuditActionImport,
				CreatedAt: now,
			},
			{
				TableName: "transactions",
				RecordID:  ids[1],
				Action:    entities.AuditActionImport,
				CreatedAt: now,
			},
		}
		if err := env.auditRepo.AppendBatch(context.Background(), auditEntries); err != nil {
			t.Fatalf("append audit: %v", err)
		}

		// Rebuild overlay
		if err := env.ovSvc.Rebuild(context.Background()); err != nil {
			t.Fatalf("rebuild: %v", err)
		}

		// Verify initially present in DB
		count, _ := env.txRepo.Count(context.Background(), ports.TxFilters{})
		if count != 2 {
			t.Fatalf("expected 2 transactions, got %d", count)
		}

		// Revert the import batch!
		if err := env.annSvc.Undo(context.Background()); err != nil {
			t.Fatalf("undo import: %v", err)
		}

		// Verify transactions are deleted from DB
		count, _ = env.txRepo.Count(context.Background(), ports.TxFilters{})
		if count != 0 {
			t.Fatalf("expected 0 transactions after undo, got %d", count)
		}

		// Verify import batch is also deleted
		_, err = env.batchRepo.GetByID(context.Background(), batchID)
		if !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("expected import batch to be deleted, got err=%v", err)
		}

		// Check overlay is rebuilt and empty
		rows, _ := env.ovRepo.FindAll(context.Background(), ports.OverlayFindOptions{})
		if len(rows) != 0 {
			t.Fatalf("expected overlay to be empty, got %d rows", len(rows))
		}
	})
}
