package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

func TestRuleApplySetsCategoryBucketTag(t *testing.T) {
	ctx := context.Background()
	db, err := persistence.Open(ctx, tempDBPath(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	txRepo := persistence.NewTransactionRepository(db)
	tagRepo := persistence.NewTagRepository(db)
	bucketRepo := persistence.NewBucketRepository(db)
	categoryRepo := persistence.NewCategoryRepository(db)
	ruleRepo := persistence.NewRuleRepository(db)
	auditRepo := persistence.NewAuditLogRepository(db)
	batchRepo := persistence.NewImportBatchRepository(db)
	overlaySvc := services.NewOverlayService(db.DB)

	if _, err := db.Exec(`INSERT INTO categories (name) VALUES ('need')`); err != nil {
		t.Fatalf("seed need: %v", err)
	}
	needCat, _ := categoryRepo.GetByName(ctx, "need")

	bid, _ := bucketRepo.Create(ctx, &entities.Bucket{
		Name: "groceries", Currency: "EUR", MonthlyAllocationMinor: 30000,
	})
	txID, _ := txRepo.Insert(ctx, &entities.Transaction{
		EffectiveDate: "2026-06-20",
		Amount:        valueobjects.MustNew(-4210, valueobjects.EUR),
		Description:   "Grocery store",
		SourceHash:    "h1",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})

	desc := "grocer"
	cat := "need"
	_, err = ruleRepo.Create(ctx, &entities.Rule{
		Name:             "groceries",
		Priority:         10,
		MatchDescription: &desc,
		SetCategory:      &cat,
		SetBucketID:      &bid,
		AddTags:          []string{"groceries"},
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	annSvc := services.NewAnnotationService(services.AnnotationDeps{
		DB: db.DB, TxRepo: txRepo, TagRepo: tagRepo, BucketRepo: bucketRepo,
		CategoryRepo: categoryRepo,
		AuditRepo: auditRepo, BatchRepo: batchRepo, OverlaySvc: overlaySvc,
	})
	ruleSvc := services.NewRuleService(services.RuleDeps{
		TxRepo: txRepo, TagRepo: tagRepo, BucketRepo: bucketRepo,
		CategoryRepo: categoryRepo,
		RuleRepo: ruleRepo, AnnService: annSvc,
	})
	result, err := ruleSvc.Apply(ctx, false)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Applied != 1 {
		t.Fatalf("applied = %d, want 1", result.Applied)
	}

	txn, _ := txRepo.GetByID(ctx, txID)
	if txn.CategoryID == nil || *txn.CategoryID != needCat.ID {
		t.Errorf("category = %v, want id %d", txn.CategoryID, needCat.ID)
	}
	if txn.BucketID == nil || *txn.BucketID != bid {
		t.Errorf("bucket = %v, want %d", txn.BucketID, bid)
	}
	tags, _ := tagRepo.ListByTransaction(ctx, txID)
	if len(tags) != 1 || tags[0] != "groceries" {
		t.Errorf("tags = %v, want [groceries]", tags)
	}
}

func TestRuleApplyNoOverwrite(t *testing.T) {
	ctx := context.Background()
	db, err := persistence.Open(ctx, tempDBPath(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	txRepo := persistence.NewTransactionRepository(db)
	tagRepo := persistence.NewTagRepository(db)
	bucketRepo := persistence.NewBucketRepository(db)
	categoryRepo := persistence.NewCategoryRepository(db)
	ruleRepo := persistence.NewRuleRepository(db)
	auditRepo := persistence.NewAuditLogRepository(db)
	batchRepo := persistence.NewImportBatchRepository(db)
	overlaySvc := services.NewOverlayService(db.DB)

	if _, err := db.Exec(`INSERT INTO categories (name) VALUES ('want'), ('need')`); err != nil {
		t.Fatalf("seed want+need: %v", err)
	}
	wantCat, _ := categoryRepo.GetByName(ctx, "want")

	txID, _ := txRepo.Insert(ctx, &entities.Transaction{
		EffectiveDate: "2026-06-20",
		Amount:        valueobjects.MustNew(-1000, valueobjects.EUR),
		Description:   "Coffee at corner",
		SourceHash:    "h",
		CategoryID:    &wantCat.ID, // already set
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})

	desc := "corner"
	cat := "need" // would try to change want -> need
	_, err = ruleRepo.Create(ctx, &entities.Rule{
		Name: "force-coffee-to-need",
		MatchDescription: &desc,
		SetCategory:      &cat,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	annSvc := services.NewAnnotationService(services.AnnotationDeps{
		DB: db.DB, TxRepo: txRepo, TagRepo: tagRepo, BucketRepo: bucketRepo,
		CategoryRepo: categoryRepo,
		AuditRepo: auditRepo, BatchRepo: batchRepo, OverlaySvc: overlaySvc,
	})
	ruleSvc := services.NewRuleService(services.RuleDeps{
		TxRepo: txRepo, TagRepo: tagRepo, BucketRepo: bucketRepo,
		CategoryRepo: categoryRepo,
		RuleRepo: ruleRepo, AnnService: annSvc,
	})
	result, err := ruleSvc.Apply(ctx, false)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Matched != 1 || result.Applied != 0 {
		t.Fatalf("expected 1 matched / 0 applied, got %+v", result)
	}
	txn, _ := txRepo.GetByID(ctx, txID)
	if txn.CategoryID == nil || *txn.CategoryID != wantCat.ID {
		t.Fatalf("category was overwritten: %v", txn.CategoryID)
	}
}

func tempDBPath(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/ledger.db"
}

func TestRuleApplyOverwriteChangesExistingCategory(t *testing.T) {
	ctx := context.Background()
	db, err := persistence.Open(ctx, tempDBPath(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	txRepo := persistence.NewTransactionRepository(db)
	tagRepo := persistence.NewTagRepository(db)
	bucketRepo := persistence.NewBucketRepository(db)
	categoryRepo := persistence.NewCategoryRepository(db)
	ruleRepo := persistence.NewRuleRepository(db)
	auditRepo := persistence.NewAuditLogRepository(db)
	batchRepo := persistence.NewImportBatchRepository(db)
	overlaySvc := services.NewOverlayService(db.DB)

	if _, err := db.Exec(`INSERT INTO categories (name) VALUES ('want'), ('need')`); err != nil {
		t.Fatalf("seed want+need: %v", err)
	}
	wantCat, _ := categoryRepo.GetByName(ctx, "want")
	needCat, _ := categoryRepo.GetByName(ctx, "need")

	txID, _ := txRepo.Insert(ctx, &entities.Transaction{
		EffectiveDate: "2026-06-20",
		Amount:        valueobjects.MustNew(-1000, valueobjects.EUR),
		Description:   "Coffee at corner",
		SourceHash:    "h",
		CategoryID:    &wantCat.ID, // already categorized
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})

	desc := "corner"
	cat := "need"
	_, err = ruleRepo.Create(ctx, &entities.Rule{
		Name:             "corner-coffee-to-need",
		MatchDescription: &desc,
		SetCategory:      &cat,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	annSvc := services.NewAnnotationService(services.AnnotationDeps{
		DB: db.DB, TxRepo: txRepo, TagRepo: tagRepo, BucketRepo: bucketRepo,
		CategoryRepo: categoryRepo,
		AuditRepo: auditRepo, BatchRepo: batchRepo, OverlaySvc: overlaySvc,
	})
	ruleSvc := services.NewRuleService(services.RuleDeps{
		TxRepo: txRepo, TagRepo: tagRepo, BucketRepo: bucketRepo,
		CategoryRepo: categoryRepo,
		RuleRepo: ruleRepo, AnnService: annSvc,
	})

	// Without --overwrite: rule is skipped (already categorized).
	result, err := ruleSvc.Apply(ctx, false)
	if err != nil {
		t.Fatalf("apply no-overwrite: %v", err)
	}
	if result.Matched != 1 || result.Applied != 0 {
		t.Fatalf("expected 1 matched / 0 applied, got %+v", result)
	}
	txn, _ := txRepo.GetByID(ctx, txID)
	if txn.CategoryID == nil || *txn.CategoryID != wantCat.ID {
		t.Fatalf("expected unchanged category (id=%d), got %v", wantCat.ID, txn.CategoryID)
	}

	// With --overwrite: rule applies, category flips to "need".
	result, err = ruleSvc.Apply(ctx, true)
	if err != nil {
		t.Fatalf("apply overwrite: %v", err)
	}
	if result.Matched != 1 || result.Applied != 1 {
		t.Fatalf("expected 1 matched / 1 applied, got %+v", result)
	}
	txn, _ = txRepo.GetByID(ctx, txID)
	if txn.CategoryID == nil || *txn.CategoryID != needCat.ID {
		t.Fatalf("expected category id=%d after overwrite, got %v", needCat.ID, txn.CategoryID)
	}

	// Audit log has a rule_apply row.
	entries, _ := auditRepo.Query(ctx, ports.AuditEntryFilter{
		TableName: strPtr("transactions"),
		RecordID:  &txID,
		Action:    strPtr(entities.AuditActionRuleApply),
	})
	if len(entries) != 1 {
		t.Fatalf("expected 1 rule_apply row, got %d", len(entries))
	}
	if entries[0].NewValue == nil || *entries[0].NewValue != "need" {
		t.Fatalf("new value = %v, want \"need\"", entries[0].NewValue)
	}
	if entries[0].OldValue == nil || *entries[0].OldValue != "want" {
		t.Fatalf("old value = %v, want \"want\"", entries[0].OldValue)
	}
}

func TestUndoRuleApplyRestoresCategory(t *testing.T) {
	ctx := context.Background()
	db, err := persistence.Open(ctx, tempDBPath(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	txRepo := persistence.NewTransactionRepository(db)
	tagRepo := persistence.NewTagRepository(db)
	bucketRepo := persistence.NewBucketRepository(db)
	categoryRepo := persistence.NewCategoryRepository(db)
	ruleRepo := persistence.NewRuleRepository(db)
	auditRepo := persistence.NewAuditLogRepository(db)
	batchRepo := persistence.NewImportBatchRepository(db)
	overlaySvc := services.NewOverlayService(db.DB)

	if _, err := db.Exec(`INSERT INTO categories (name) VALUES ('want'), ('need')`); err != nil {
		t.Fatalf("seed want+need: %v", err)
	}
	wantCat, _ := categoryRepo.GetByName(ctx, "want")

	txID, _ := txRepo.Insert(ctx, &entities.Transaction{
		EffectiveDate: "2026-06-20",
		Amount:        valueobjects.MustNew(-1000, valueobjects.EUR),
		Description:   "Coffee at corner",
		SourceHash:    "h",
		CategoryID:    &wantCat.ID,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})

	desc := "corner"
	cat := "need"
	_, _ = ruleRepo.Create(ctx, &entities.Rule{
		Name: "corner-coffee", MatchDescription: &desc, SetCategory: &cat, Enabled: true,
	})

	annSvc := services.NewAnnotationService(services.AnnotationDeps{
		DB: db.DB, TxRepo: txRepo, TagRepo: tagRepo, BucketRepo: bucketRepo,
		CategoryRepo: categoryRepo,
		AuditRepo: auditRepo, BatchRepo: batchRepo, OverlaySvc: overlaySvc,
	})
	ruleSvc := services.NewRuleService(services.RuleDeps{
		TxRepo: txRepo, TagRepo: tagRepo, BucketRepo: bucketRepo,
		CategoryRepo: categoryRepo,
		RuleRepo: ruleRepo, AnnService: annSvc,
	})
	if _, err := ruleSvc.Apply(ctx, true); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := annSvc.Undo(ctx); err != nil {
		t.Fatalf("undo: %v", err)
	}
	txn, _ := txRepo.GetByID(ctx, txID)
	if txn.CategoryID == nil || *txn.CategoryID != wantCat.ID {
		t.Fatalf("expected category restored to id=%d, got %v", wantCat.ID, txn.CategoryID)
	}
}
