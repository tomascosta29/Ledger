package persistence_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

func newTestDB(t *testing.T) *persistence.DB {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.db")
	db, err := persistence.Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedCategory(t *testing.T, db *persistence.DB, name string) int64 {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO categories (name) VALUES (?)`, name); err != nil {
		t.Fatalf("seed category %q: %v", name, err)
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM categories WHERE name = ?`, name).Scan(&id); err != nil {
		t.Fatalf("lookup seeded category %q: %v", name, err)
	}
	return id
}

func categoryName(t *testing.T, db *persistence.DB, id *int64) string {
	t.Helper()
	if id == nil {
		return ""
	}
	c, err := persistence.NewCategoryRepository(db).GetByID(context.Background(), *id)
	if err != nil {
		t.Fatalf("category lookup: %v", err)
	}
	return c.Name
}

func TestOpenAndMigrate(t *testing.T) {
	db := newTestDB(t)

	var schemaVersion int64
	if err := db.QueryRow(`SELECT MAX(version_id) FROM goose_db_version`).Scan(&schemaVersion); err != nil {
		t.Fatalf("query schema version: %v", err)
	}
	if schemaVersion < 1 {
		t.Fatalf("expected schema version >= 1, got %d", schemaVersion)
	}
}

func TestTransactionRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	repo := persistence.NewTransactionRepository(db)

	partner := "ACME GmbH"
	desc := "Invoice 42"
	now := time.Now().UTC().Truncate(time.Millisecond)

	tx := &entities.Transaction{
		EffectiveDate: "2026-06-15",
		Amount:        valueobjects.MustNew(-12345, valueobjects.EUR),
		Description:   desc,
		PartnerName:   &partner,
		SourceHash:    "abc123",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	id, err := repo.Insert(ctx, tx)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Amount.Amount != -12345 || got.Amount.Currency != valueobjects.EUR {
		t.Fatalf("amount mismatch: got %+v", got.Amount)
	}
	if got.EffectiveDate != "2026-06-15" {
		t.Fatalf("date mismatch: %q", got.EffectiveDate)
	}
	if got.PartnerName == nil || *got.PartnerName != partner {
		t.Fatalf("partner mismatch: %+v", got.PartnerName)
	}
	if got.CategoryID != nil {
		t.Fatalf("category default should be uncategorized, got id %v", *got.CategoryID)
	}
	if got.IsHidden || got.ExcludeFromReports {
		t.Fatalf("defaults wrong: hidden=%v exclude=%v", got.IsHidden, got.ExcludeFromReports)
	}
}

func TestSourceHashDedupe(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := persistence.NewTransactionRepository(db)

	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		tx := &entities.Transaction{
			EffectiveDate: "2026-06-15",
			Amount:        valueobjects.MustNew(-100, valueobjects.EUR),
			SourceHash:    "duplicate-hash",
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if _, err := repo.Insert(ctx, tx); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	got, err := repo.GetBySourceHash(ctx, "duplicate-hash")
	if err != nil {
		t.Fatalf("get by hash: %v", err)
	}
	if got.SourceHash != "duplicate-hash" {
		t.Fatalf("hash lookup wrong: %q", got.SourceHash)
	}
}

func TestFindAllFilters(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := persistence.NewTransactionRepository(db)
	now := time.Now().UTC()

	mk := func(date string, amountMinor int64, hidden bool) {
		t.Helper()
		_, err := repo.Insert(ctx, &entities.Transaction{
			EffectiveDate: date,
			Amount:        valueobjects.MustNew(amountMinor, valueobjects.EUR),
			SourceHash:    date + "-" + string(rune('a'+amountMinor)),
			IsHidden:      hidden,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	mk("2026-01-15", -1000, false)
	mk("2026-02-15", -2000, true)
	mk("2026-03-15", 5000, false)

	t.Run("no filters", func(t *testing.T) {
		got, err := repo.FindAll(ctx, ports.TxFindOptions{})
		if err != nil {
			t.Fatalf("find all: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("want 3, got %d", len(got))
		}
	})

	t.Run("hidden=false only", func(t *testing.T) {
		hidden := false
		got, err := repo.FindAll(ctx, ports.TxFindOptions{Filters: ports.TxFilters{IsHidden: &hidden}})
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("want 2 visible, got %d", len(got))
		}
	})

	t.Run("date range", func(t *testing.T) {
		start, end := "2026-02-01", "2026-02-28"
		got, err := repo.FindAll(ctx, ports.TxFindOptions{Filters: ports.TxFilters{StartDate: &start, EndDate: &end}})
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("want 1 in Feb, got %d", len(got))
		}
	})

	t.Run("sign positive only", func(t *testing.T) {
		sign := "positive"
		got, err := repo.FindAll(ctx, ports.TxFindOptions{Filters: ports.TxFilters{AmountSign: &sign}})
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("want 1 positive, got %d", len(got))
		}
		if !got[0].IsIncome() {
			t.Fatal("expected income tx")
		}
	})

	t.Run("amount range minor units", func(t *testing.T) {
		min, max := int64(-1500), int64(-500)
		got, err := repo.FindAll(ctx, ports.TxFindOptions{Filters: ports.TxFilters{AmountMinMinor: &min, AmountMaxMinor: &max}})
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("want 1 in range, got %d", len(got))
		}
	})
}

func TestUpdateFields(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := persistence.NewTransactionRepository(db)
	now := time.Now().UTC()

	wantID := seedCategory(t, db, "want")
	id, err := repo.Insert(ctx, &entities.Transaction{
		EffectiveDate: "2026-06-15",
		Amount:        valueobjects.MustNew(-100, valueobjects.EUR),
		SourceHash:    "h",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := repo.SetHidden(ctx, id, true); err != nil {
		t.Fatalf("set hidden: %v", err)
	}
	if err := repo.SetCategory(ctx, id, &wantID); err != nil {
		t.Fatalf("set category: %v", err)
	}
	if err := repo.SetExcludeFromReports(ctx, id, true); err != nil {
		t.Fatalf("set exclude: %v", err)
	}

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.IsHidden {
		t.Fatal("expected hidden=true")
	}
	if !got.ExcludeFromReports {
		t.Fatal("expected exclude_from_reports=true")
	}
	if categoryName(t, db, got.CategoryID) != "want" {
		t.Fatalf("category wrong: %v", got.CategoryID)
	}
}

func TestAuditLogRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	auditRepo := persistence.NewAuditLogRepository(db)
	txRepo := persistence.NewTransactionRepository(db)
	now := time.Now().UTC().Truncate(time.Millisecond)

	id, err := txRepo.Insert(ctx, &entities.Transaction{
		EffectiveDate: "2026-06-15",
		Amount:        valueobjects.MustNew(-100, valueobjects.EUR),
		SourceHash:    "h",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("insert tx: %v", err)
	}

	field := "category"
	oldVal := "Unknown"
	newVal := "want"
	entries := []*entities.AuditEntry{
		{TableName: "transactions", RecordID: id, Action: entities.AuditActionCategorize, Field: &field, OldValue: &oldVal, NewValue: &newVal, CreatedAt: now},
		{TableName: "transactions", RecordID: id, Action: entities.AuditActionCategorize, Field: &field, OldValue: &oldVal, NewValue: &newVal, CreatedAt: now},
	}
	if err := auditRepo.AppendBatch(ctx, entries); err != nil {
		t.Fatalf("append batch: %v", err)
	}

	got, err := auditRepo.LastBatch(ctx, "transactions", id)
	if err != nil {
		t.Fatalf("last batch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 audit entries in last batch, got %d", len(got))
	}

	later := now.Add(time.Second)
	if _, err := auditRepo.Append(ctx, &entities.AuditEntry{
		TableName: "transactions", RecordID: id, Action: entities.AuditActionEdit,
		Field: &field, OldValue: &oldVal, NewValue: &newVal, CreatedAt: later,
	}); err != nil {
		t.Fatalf("append single: %v", err)
	}

	got, err = auditRepo.LastBatch(ctx, "transactions", id)
	if err != nil {
		t.Fatalf("last batch (after): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entry in latest batch, got %d", len(got))
	}
	if got[0].CreatedAt.Sub(later).Abs() > time.Millisecond {
		t.Fatalf("created_at not matching: got %v, want %v", got[0].CreatedAt, later)
	}
}

func TestImportBatchRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := persistence.NewImportBatchRepository(db)

	id, err := repo.Create(ctx, &entities.ImportBatch{
		SourceFile:    "erste.csv",
		SourceProfile: "erste",
		RowCount:      100,
		InsertedCount: 95,
		SkippedCount:  5,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	if err := repo.UpdateCounts(ctx, id, 97, 3); err != nil {
		t.Fatalf("update counts: %v", err)
	}

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.InsertedCount != 97 || got.SkippedCount != 3 {
		t.Fatalf("counts wrong: %+v", got)
	}
	if got.SourceProfile != "erste" {
		t.Fatalf("profile wrong: %q", got.SourceProfile)
	}
}

func TestInitIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.db")

	ctx := context.Background()
	db1, err := persistence.Open(ctx, path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := persistence.Migrate(db1.DB); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	_ = db1.Close()

	db2, err := persistence.Open(ctx, path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer db2.Close()
	if err := persistence.Migrate(db2.DB); err != nil {
		t.Fatalf("second migrate (idempotent check): %v", err)
	}
}

func TestBucketCRUD(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := persistence.NewBucketRepository(db)

	id, err := repo.Create(ctx, &entities.Bucket{
		Name: "vacation-2026", Currency: "EUR", MonthlyAllocationMinor: 50000,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "vacation-2026" || got.Currency != "EUR" || got.MonthlyAllocationMinor != 50000 {
		t.Fatalf("bucket wrong: %+v", got)
	}

	byName, err := repo.GetByName(ctx, "vacation-2026")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if byName.ID != id {
		t.Fatalf("get by name returned wrong id")
	}

	all, err := repo.List(ctx, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1, got %d", len(all))
	}

	if err := repo.Archive(ctx, id); err != nil {
		t.Fatalf("archive: %v", err)
	}
	active, _ := repo.List(ctx, false)
	if len(active) != 0 {
		t.Fatalf("expected 0 active after archive, got %d", len(active))
	}
	allIncluding, _ := repo.List(ctx, true)
	if len(allIncluding) != 1 {
		t.Fatalf("expected 1 including archived, got %d", len(allIncluding))
	}
}

func TestBucketDeleteBlockedByAssignment(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	bucketRepo := persistence.NewBucketRepository(db)
	txRepo := persistence.NewTransactionRepository(db)

	bucketID, err := bucketRepo.Create(ctx, &entities.Bucket{
		Name: "rent", Currency: "EUR", MonthlyAllocationMinor: 80000,
	})
	if err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	txID, err := txRepo.Insert(ctx, &entities.Transaction{
		EffectiveDate: "2026-06-20",
		Amount:        valueobjects.MustNew(-150000, valueobjects.EUR),
		Description:   "rent",
		SourceHash:    "h",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert tx: %v", err)
	}
	if err := txRepo.SetBucket(ctx, txID, bucketID); err != nil {
		t.Fatalf("assign bucket: %v", err)
	}

	if err := bucketRepo.Delete(ctx, bucketID); err == nil {
		t.Fatal("expected delete to fail with assigned transaction")
	}
	if err := bucketRepo.Archive(ctx, bucketID); err != nil {
		t.Fatalf("archive should still work: %v", err)
	}
}

func TestBucketSpendByMonth(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	bucketRepo := persistence.NewBucketRepository(db)
	txRepo := persistence.NewTransactionRepository(db)

	bucketID, _ := bucketRepo.Create(ctx, &entities.Bucket{
		Name: "groceries", Currency: "EUR", MonthlyAllocationMinor: 30000,
	})

	for i, amount := range []int64{-4200, -1500, -8000} {
		txID, err := txRepo.Insert(ctx, &entities.Transaction{
			EffectiveDate: fmt.Sprintf("2026-06-%02d", 10+i),
			Amount:        valueobjects.MustNew(amount, valueobjects.EUR),
			Description:   fmt.Sprintf("tx %d", i),
			SourceHash:    fmt.Sprintf("h%d", i),
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		if err := txRepo.SetBucket(ctx, txID, bucketID); err != nil {
			t.Fatalf("assign %d: %v", i, err)
		}
	}
	_ = txRepo

	spends, err := bucketRepo.SpendByMonth(ctx, "2026-06")
	if err != nil {
		t.Fatalf("spend: %v", err)
	}
	if len(spends) != 1 {
		t.Fatalf("expected 1 row, got %d", len(spends))
	}
	if spends[0].SpentMinor != 13700 {
		t.Fatalf("spent = %d, want 13700", spends[0].SpentMinor)
	}
	if spends[0].Count != 3 {
		t.Fatalf("count = %d, want 3", spends[0].Count)
	}

	unassigned, err := bucketRepo.UnassignedSpendByMonth(ctx, "2026-06")
	if err != nil {
		t.Fatalf("unassigned: %v", err)
	}
	if len(unassigned) != 0 {
		t.Fatalf("expected 0 unassigned, got %d", len(unassigned))
	}
}
