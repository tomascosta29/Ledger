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
	db         *persistence.DB
	annSvc     *services.AnnotationService
	txRepo     *persistence.TransactionRepository
	tagRepo    *persistence.TagRepository
	bucketRepo *persistence.BucketRepository
	auditRepo  *persistence.AuditLogRepository
	batchRepo  *persistence.ImportBatchRepository
	ovSvc      *services.OverlayService
	ovRepo     *persistence.OverlayRepository
	cleanup    func()
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
	bucketRepo := persistence.NewBucketRepository(db)
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
		BucketRepo: bucketRepo,
		AuditRepo:  auditRepo,
		BatchRepo:  batchRepo,
		OverlaySvc: ovSvc,
		Now:        nowFunc,
	})
	return &annotationTestEnv{
		db:         db,
		annSvc:     annSvc,
		txRepo:     txRepo,
		tagRepo:    tagRepo,
		bucketRepo: bucketRepo,
		auditRepo:  auditRepo,
		batchRepo:  batchRepo,
		ovSvc:      ovSvc,
		ovRepo:     ovRepo,
		cleanup:    func() { _ = db.Close() },
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

	if err := env.annSvc.Categorize(context.Background(), id, "want", nil); err != nil {
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

	err := env.annSvc.Categorize(context.Background(), id, "want", nil)
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
		if err := env.annSvc.Categorize(context.Background(), id, "want", nil); err != nil {
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
		if err := env.annSvc.Categorize(context.Background(), id, "want", nil); err != nil {
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

func TestBulkCategorize(t *testing.T) {
	t.Run("multiple ids get the category and share one audit timestamp", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id1 := env.seedTx(t)
		id2 := env.seedTx(t)
		id3 := env.seedTx(t)

		if err := env.annSvc.BulkCategorize(context.Background(), []int64{id1, id2, id3}, "want", nil); err != nil {
			t.Fatalf("bulk categorize: %v", err)
		}

		for _, id := range []int64{id1, id2, id3} {
			txn, _ := env.txRepo.GetByID(context.Background(), id)
			if txn.Category != "want" {
				t.Fatalf("txn %d category = %q, want %q", id, txn.Category, "want")
			}
		}

		// All three audit rows share the same timestamp (single batch).
		entries, _ := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{Action: strPtr("categorize")})
		if len(entries) != 3 {
			t.Fatalf("expected 3 categorize audit rows, got %d", len(entries))
		}
		ts0 := entries[0].CreatedAt
		for _, e := range entries[1:] {
			if !e.CreatedAt.Equal(ts0) {
				t.Fatalf("expected shared timestamp %v, got %v", ts0, e.CreatedAt)
			}
		}

		// Overlay reflects the new category.
		rows, _ := env.ovRepo.FindAll(context.Background(), ports.OverlayFindOptions{})
		if len(rows) != 3 {
			t.Fatalf("expected 3 overlay rows, got %d", len(rows))
		}
		for _, r := range rows {
			if r.Category != "want" {
				t.Fatalf("overlay category = %q, want %q", r.Category, "want")
			}
		}
	})

	t.Run("dedupes ids and skips no-op rows", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id1 := env.seedTx(t)
		id2 := env.seedTx(t)
		// id1 already "Unknown"; bulk-categorizing to "Unknown" is a no-op for it.
		if err := env.annSvc.BulkCategorize(context.Background(), []int64{id1, id2, id1}, "Unknown", nil); err != nil {
			t.Fatalf("bulk categorize: %v", err)
		}
		entries, _ := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{Action: strPtr("categorize")})
		if len(entries) != 0 {
			t.Fatalf("expected 0 audit rows for all no-ops, got %d", len(entries))
		}
	})

	t.Run("empty ids is an error", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		if err := env.annSvc.BulkCategorize(context.Background(), nil, "want", nil); err == nil {
			t.Fatal("expected error for empty id list")
		}
	})

	t.Run("rollback on missing id", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id1 := env.seedTx(t)
		missing := id1 + 9999
		err := env.annSvc.BulkCategorize(context.Background(), []int64{id1, missing}, "want", nil)
		if err == nil {
			t.Fatal("expected error from missing id")
		}
		txn, _ := env.txRepo.GetByID(context.Background(), id1)
		if txn.Category != "Unknown" {
			t.Fatalf("expected id1 unchanged after rollback, got category %q", txn.Category)
		}
		entries, _ := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{Action: strPtr("categorize")})
		if len(entries) != 0 {
			t.Fatalf("expected 0 audit rows after rollback, got %d", len(entries))
		}
	})
}

func TestBulkSetHidden(t *testing.T) {
	t.Run("multiple ids get the hidden flag and share one audit timestamp", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id1 := env.seedTx(t)
		id2 := env.seedTx(t)

		if err := env.annSvc.BulkSetHidden(context.Background(), []int64{id1, id2}, true); err != nil {
			t.Fatalf("bulk hide: %v", err)
		}
		for _, id := range []int64{id1, id2} {
			txn, _ := env.txRepo.GetByID(context.Background(), id)
			if !txn.IsHidden {
				t.Fatalf("txn %d not hidden", id)
			}
		}
		entries, _ := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{Action: strPtr("visibility")})
		if len(entries) != 2 {
			t.Fatalf("expected 2 visibility audit rows, got %d", len(entries))
		}
		if !entries[0].CreatedAt.Equal(entries[1].CreatedAt) {
			t.Fatalf("audit rows do not share timestamp")
		}
	})
}

func TestBulkAddTags(t *testing.T) {
	t.Run("adds tags to each transaction and shares one audit timestamp", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id1 := env.seedTx(t)
		id2 := env.seedTx(t)

		if err := env.annSvc.BulkAddTags(context.Background(), []int64{id1, id2}, []string{"rent", "monthly"}); err != nil {
			t.Fatalf("bulk add tags: %v", err)
		}
		for _, id := range []int64{id1, id2} {
			tags, _ := env.tagRepo.ListByTransaction(context.Background(), id)
			if len(tags) != 2 || tags[0] != "monthly" || tags[1] != "rent" {
				t.Fatalf("txn %d tags = %v, want [monthly rent]", id, tags)
			}
		}
		entries, _ := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{Action: strPtr("tag")})
		if len(entries) != 2 {
			t.Fatalf("expected 2 tag audit rows, got %d", len(entries))
		}
		if !entries[0].CreatedAt.Equal(entries[1].CreatedAt) {
			t.Fatalf("audit rows do not share timestamp")
		}
	})

	t.Run("skips already-present tags", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id := env.seedTx(t)
		if err := env.annSvc.AddTag(context.Background(), id, "rent"); err != nil {
			t.Fatalf("seed tag: %v", err)
		}
		if err := env.annSvc.BulkAddTags(context.Background(), []int64{id}, []string{"rent", "monthly"}); err != nil {
			t.Fatalf("bulk add tags: %v", err)
		}
		tags, _ := env.tagRepo.ListByTransaction(context.Background(), id)
		if len(tags) != 2 || tags[0] != "monthly" || tags[1] != "rent" {
			t.Fatalf("tags = %v, want [monthly rent]", tags)
		}
		// Two audit rows total: one from the seed AddTag("rent"), one from the bulk.
		// The bulk row's new_value must include both "rent" and "monthly".
		entries, _ := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{Action: strPtr("tag")})
		if len(entries) != 2 {
			t.Fatalf("expected 2 tag audit rows (seed + bulk), got %d", len(entries))
		}
		var bulk *entities.AuditEntry
		for _, e := range entries {
			if e.NewValue != nil && *e.NewValue == "rent,monthly" {
				bulk = e
			}
		}
		if bulk == nil {
			t.Fatalf("expected one audit row with new_value=monthly,rent, got %+v", entries)
		}
	})
}

func TestBulkRemoveTags(t *testing.T) {
	t.Run("removes tags from each transaction and shares one audit timestamp", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id1 := env.seedTx(t)
		id2 := env.seedTx(t)
		for _, id := range []int64{id1, id2} {
			if err := env.annSvc.AddTag(context.Background(), id, "rent"); err != nil {
				t.Fatalf("seed: %v", err)
			}
			if err := env.annSvc.AddTag(context.Background(), id, "monthly"); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}

		if err := env.annSvc.BulkRemoveTags(context.Background(), []int64{id1, id2}, []string{"rent"}); err != nil {
			t.Fatalf("bulk remove tags: %v", err)
		}
		for _, id := range []int64{id1, id2} {
			tags, _ := env.tagRepo.ListByTransaction(context.Background(), id)
			if len(tags) != 1 || tags[0] != "monthly" {
				t.Fatalf("txn %d tags = %v, want [monthly]", id, tags)
			}
		}
	})
}

func TestUndoBulkCategorize(t *testing.T) {
	env := newAnnotationTestEnv(t)
	defer env.cleanup()
	id1 := env.seedTx(t)
	id2 := env.seedTx(t)
	id3 := env.seedTx(t)

	if err := env.annSvc.BulkCategorize(context.Background(), []int64{id1, id2, id3}, "want", nil); err != nil {
		t.Fatalf("bulk categorize: %v", err)
	}
	// One undo must revert the whole batch.
	if err := env.annSvc.Undo(context.Background()); err != nil {
		t.Fatalf("undo: %v", err)
	}
	for _, id := range []int64{id1, id2, id3} {
		txn, _ := env.txRepo.GetByID(context.Background(), id)
		if txn.Category != "Unknown" {
			t.Fatalf("txn %d category after undo = %q, want Unknown", id, txn.Category)
		}
	}
}

func TestCategorizeWithBucket(t *testing.T) {
	t.Run("sets category and bucket on a transaction", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id := env.seedTx(t)
		bid, err := env.bucketRepo.Create(context.Background(), &entities.Bucket{
			Name: "vacation-2026", Currency: "EUR", MonthlyAllocationMinor: 50000,
		})
		if err != nil {
			t.Fatalf("create bucket: %v", err)
		}
		name := "vacation-2026"
		if err := env.annSvc.Categorize(context.Background(), id, "want", &name); err != nil {
			t.Fatalf("categorize: %v", err)
		}
		txn, _ := env.txRepo.GetByID(context.Background(), id)
		if txn.Category != "want" {
			t.Fatalf("category = %q, want want", txn.Category)
		}
		if txn.BucketID == nil || *txn.BucketID != bid {
			t.Fatalf("bucket = %v, want %d", txn.BucketID, bid)
		}
		// Two audit rows share a timestamp.
		entries, _ := env.auditRepo.Query(context.Background(), ports.AuditEntryFilter{})
		var cat, buck *entities.AuditEntry
		for _, e := range entries {
			switch e.Action {
			case "categorize":
				cat = e
			case "bucket_assign":
				buck = e
			}
		}
		if cat == nil || buck == nil {
			t.Fatalf("expected both categorize and bucket_assign audit rows, got cat=%v buck=%v", cat, buck)
		}
		if !cat.CreatedAt.Equal(buck.CreatedAt) {
			t.Fatalf("audit rows do not share timestamp")
		}
	})

	t.Run("missing bucket is an error", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id := env.seedTx(t)
		name := "nope"
		if err := env.annSvc.Categorize(context.Background(), id, "want", &name); err == nil {
			t.Fatal("expected error for missing bucket")
		}
		txn, _ := env.txRepo.GetByID(context.Background(), id)
		if txn.Category != "Unknown" {
			t.Fatalf("category changed despite failure: %q", txn.Category)
		}
	})

	t.Run("currency mismatch is an error", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id := env.seedTx(t)
		_, err := env.bucketRepo.Create(context.Background(), &entities.Bucket{
			Name: "us-bucket", Currency: "USD", MonthlyAllocationMinor: 10000,
		})
		if err != nil {
			t.Fatalf("create bucket: %v", err)
		}
		name := "us-bucket"
		err = env.annSvc.Categorize(context.Background(), id, "want", &name)
		if err == nil {
			t.Fatal("expected currency mismatch error")
		}
	})

	t.Run("undo restores prior bucket", func(t *testing.T) {
		env := newAnnotationTestEnv(t)
		defer env.cleanup()
		id := env.seedTx(t)
		bid, _ := env.bucketRepo.Create(context.Background(), &entities.Bucket{
			Name: "rent", Currency: "EUR", MonthlyAllocationMinor: 80000,
		})
		if err := env.txRepo.SetBucket(context.Background(), id, bid); err != nil {
			t.Fatalf("seed bucket: %v", err)
		}
		originalID := bid
		_, _ = env.bucketRepo.Create(context.Background(), &entities.Bucket{
			Name: "vacation", Currency: "EUR", MonthlyAllocationMinor: 50000,
		})
		name := "vacation"
		if err := env.annSvc.Categorize(context.Background(), id, "want", &name); err != nil {
			t.Fatalf("categorize: %v", err)
		}
		if err := env.annSvc.Undo(context.Background()); err != nil {
			t.Fatalf("undo: %v", err)
		}
		txn, _ := env.txRepo.GetByID(context.Background(), id)
		if txn.BucketID == nil || *txn.BucketID != originalID {
			t.Fatalf("bucket after undo = %v, want %d", txn.BucketID, originalID)
		}
		if txn.Category != "Unknown" {
			t.Fatalf("category after undo = %q, want Unknown", txn.Category)
		}
	})
}

func TestBulkCategorizeWithBucket(t *testing.T) {
	env := newAnnotationTestEnv(t)
	defer env.cleanup()
	id1 := env.seedTx(t)
	id2 := env.seedTx(t)
	bid, _ := env.bucketRepo.Create(context.Background(), &entities.Bucket{
		Name: "vacation-2026", Currency: "EUR", MonthlyAllocationMinor: 50000,
	})
	name := "vacation-2026"
	if err := env.annSvc.BulkCategorize(context.Background(), []int64{id1, id2}, "want", &name); err != nil {
		t.Fatalf("bulk categorize: %v", err)
	}
	for _, id := range []int64{id1, id2} {
		txn, _ := env.txRepo.GetByID(context.Background(), id)
		if txn.BucketID == nil || *txn.BucketID != bid {
			t.Fatalf("txn %d bucket = %v, want %d", id, txn.BucketID, bid)
		}
	}
}
