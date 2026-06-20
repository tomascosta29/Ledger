package services_test

import (
	"context"
	"testing"
	"time"

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
	ruleRepo := persistence.NewRuleRepository(db)
	auditRepo := persistence.NewAuditLogRepository(db)
	batchRepo := persistence.NewImportBatchRepository(db)
	overlaySvc := services.NewOverlayService(db.DB)

	bid, _ := bucketRepo.Create(ctx, &entities.Bucket{
		Name: "groceries", Currency: "EUR", MonthlyAllocationMinor: 30000,
	})
	txID, _ := txRepo.Insert(ctx, &entities.Transaction{
		EffectiveDate: "2026-06-20",
		Amount:        valueobjects.MustNew(-4210, valueobjects.EUR),
		Description:   "Grocery store",
		SourceHash:    "h1",
		Category:      "Unknown",
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
		AuditRepo: auditRepo, BatchRepo: batchRepo, OverlaySvc: overlaySvc,
	})
	ruleSvc := services.NewRuleService(services.RuleDeps{
		TxRepo: txRepo, TagRepo: tagRepo, BucketRepo: bucketRepo,
		RuleRepo: ruleRepo, AnnService: annSvc,
	})
	result, err := ruleSvc.Apply(ctx)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Applied != 1 {
		t.Fatalf("applied = %d, want 1", result.Applied)
	}

	txn, _ := txRepo.GetByID(ctx, txID)
	if txn.Category != "need" {
		t.Errorf("category = %q", txn.Category)
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
	ruleRepo := persistence.NewRuleRepository(db)
	auditRepo := persistence.NewAuditLogRepository(db)
	batchRepo := persistence.NewImportBatchRepository(db)
	overlaySvc := services.NewOverlayService(db.DB)

	txID, _ := txRepo.Insert(ctx, &entities.Transaction{
		EffectiveDate: "2026-06-20",
		Amount:        valueobjects.MustNew(-1000, valueobjects.EUR),
		Description:   "Coffee at corner",
		SourceHash:    "h",
		Category:      "want", // already set
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
		AuditRepo: auditRepo, BatchRepo: batchRepo, OverlaySvc: overlaySvc,
	})
	ruleSvc := services.NewRuleService(services.RuleDeps{
		TxRepo: txRepo, TagRepo: tagRepo, BucketRepo: bucketRepo,
		RuleRepo: ruleRepo, AnnService: annSvc,
	})
	result, err := ruleSvc.Apply(ctx)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Matched != 1 || result.Applied != 0 {
		t.Fatalf("expected 1 matched / 0 applied, got %+v", result)
	}
	txn, _ := txRepo.GetByID(ctx, txID)
	if txn.Category != "want" {
		t.Fatalf("category was overwritten: %q", txn.Category)
	}
}

func tempDBPath(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/ledger.db"
}
