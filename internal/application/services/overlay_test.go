package services_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

type overlayTestEnv struct {
	db      *persistence.DB
	svc     *services.OverlayService
	txRepo  *persistence.TransactionRepository
	ovRepo  *persistence.OverlayRepository
	cleanup func()
}

func newOverlayTestEnv(t *testing.T) *overlayTestEnv {
	t.Helper()
	db, err := persistence.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &overlayTestEnv{
		db:      db,
		svc:     services.NewOverlayService(db.DB),
		txRepo:  persistence.NewTransactionRepository(db),
		ovRepo:  persistence.NewOverlayRepository(db),
		cleanup: func() { _ = db.Close() },
	}
}

func seedTx(t *testing.T, repo *persistence.TransactionRepository, date string, amountMinor int64, hidden, exclude bool) int64 {
	t.Helper()
	id, err := repo.Insert(context.Background(), &entities.Transaction{
		EffectiveDate:      date,
		Amount:             valueobjects.MustNew(amountMinor, valueobjects.EUR),
		Description:        "test",
		SourceHash:         date + "-" + string(rune('a'+amountMinor)),
		IsHidden:           hidden,
		ExcludeFromReports: exclude,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed tx: %v", err)
	}
	return id
}

func TestRebuildEmpty(t *testing.T) {
	env := newOverlayTestEnv(t)
	defer env.cleanup()

	if err := env.svc.Rebuild(context.Background()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	n, err := env.ovRepo.Count(context.Background(), ports.OverlayFilters{})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected empty overlay, got %d rows", n)
	}
}

func TestRebuildImportsBecomeRaw(t *testing.T) {
	env := newOverlayTestEnv(t)
	defer env.cleanup()

	seedTx(t, env.txRepo, "2026-04-30", -3090, false, false)
	seedTx(t, env.txRepo, "2026-05-01", 250000, false, false)
	seedTx(t, env.txRepo, "2026-05-15", -5000, false, true)

	if err := env.svc.Rebuild(context.Background()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	rows, err := env.ovRepo.FindAll(context.Background(), ports.OverlayFindOptions{})
	if err != nil {
		t.Fatalf("find all: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 overlay rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.SourceKind != ports.SourceRaw {
			t.Errorf("expected source_kind=raw, got %q", r.SourceKind)
		}
	}
}

func TestRebuildHiddenExcluded(t *testing.T) {
	env := newOverlayTestEnv(t)
	defer env.cleanup()

	seedTx(t, env.txRepo, "2026-04-30", -3090, false, false)
	hiddenID := seedTx(t, env.txRepo, "2026-05-01", -1000, true, false)

	if err := env.svc.Rebuild(context.Background()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	rows, err := env.ovRepo.FindAll(context.Background(), ports.OverlayFindOptions{})
	if err != nil {
		t.Fatalf("find all: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("hidden txn should be excluded; got %d rows", len(rows))
	}
	for _, r := range rows {
		if r.RawTransactionID != nil && *r.RawTransactionID == hiddenID {
			t.Fatalf("hidden txn leaked into overlay: %+v", r)
		}
	}
}

func TestRebuildExcludeFromReportsPropagated(t *testing.T) {
	env := newOverlayTestEnv(t)
	defer env.cleanup()

	seedTx(t, env.txRepo, "2026-04-30", -3090, false, false)
	seedTx(t, env.txRepo, "2026-05-01", 1000, false, true)

	if err := env.svc.Rebuild(context.Background()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	rows, err := env.ovRepo.FindAll(context.Background(), ports.OverlayFindOptions{})
	if err != nil {
		t.Fatalf("find all: %v", err)
	}
	excluded := 0
	for _, r := range rows {
		if r.ExcludeFromReports {
			excluded++
		}
	}
	if excluded != 1 {
		t.Fatalf("want 1 exclude_from_reports=true in overlay, got %d", excluded)
	}
}

func TestRebuildIdempotent(t *testing.T) {
	env := newOverlayTestEnv(t)
	defer env.cleanup()

	seedTx(t, env.txRepo, "2026-04-30", -3090, false, false)
	seedTx(t, env.txRepo, "2026-05-01", 1000, false, false)

	for i := 0; i < 3; i++ {
		if err := env.svc.Rebuild(context.Background()); err != nil {
			t.Fatalf("rebuild %d: %v", i, err)
		}
	}
	n, _ := env.ovRepo.Count(context.Background(), ports.OverlayFilters{})
	if n != 2 {
		t.Fatalf("expected 2 rows after 3 rebuilds, got %d", n)
	}
}

func TestRebuildClearsStaleRows(t *testing.T) {
	env := newOverlayTestEnv(t)
	defer env.cleanup()

	id := seedTx(t, env.txRepo, "2026-04-30", -3090, false, false)
	if err := env.svc.Rebuild(context.Background()); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}

	if err := env.txRepo.SetHidden(context.Background(), id, true); err != nil {
		t.Fatalf("hide: %v", err)
	}
	if err := env.svc.Rebuild(context.Background()); err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	rows, err := env.ovRepo.FindAll(context.Background(), ports.OverlayFindOptions{})
	if err != nil {
		t.Fatalf("find all: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows after hiding, got %d", len(rows))
	}
}

func TestRebuildSplitParentAndChildren(t *testing.T) {
	env := newOverlayTestEnv(t)
	defer env.cleanup()

	ctx := context.Background()
	parentID := seedTx(t, env.txRepo, "2026-04-30", -10000, false, false)
	parentIDPtr := parentID

	childA := &entities.Transaction{
		EffectiveDate: "2026-04-30",
		Amount:        valueobjects.MustNew(-6000, valueobjects.EUR),
		Description:   "groceries portion",
		ParentTxnID:   &parentIDPtr,
		SourceHash:    "child-a",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	childB := &entities.Transaction{
		EffectiveDate: "2026-04-30",
		Amount:        valueobjects.MustNew(-4000, valueobjects.EUR),
		Description:   "household portion",
		ParentTxnID:   &parentIDPtr,
		SourceHash:    "child-b",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if _, err := env.txRepo.InsertBatch(ctx, []*entities.Transaction{childA, childB}); err != nil {
		t.Fatalf("insert children: %v", err)
	}

	if err := env.svc.Rebuild(ctx); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	rows, err := env.ovRepo.FindAll(ctx, ports.OverlayFindOptions{})
	if err != nil {
		t.Fatalf("find all: %v", err)
	}

	var header, childRow int
	for _, r := range rows {
		switch r.SourceKind {
		case ports.SourceSplitHeader:
			header++
			if r.Amount.Amount != -10000 {
				t.Errorf("header amount = %d, want -10000", r.Amount.Amount)
			}
			if r.RawTransactionID == nil || *r.RawTransactionID != parentID {
				t.Errorf("header raw_transaction_id = %v, want %d", r.RawTransactionID, parentID)
			}
		case ports.SourceSplitChild:
			childRow++
			if r.ParentOverlayID == nil {
				t.Error("split_child has nil parent_overlay_id")
			}
		}
	}
	if header != 1 {
		t.Errorf("want 1 split_header, got %d", header)
	}
	if childRow != 2 {
		t.Errorf("want 2 split_child, got %d", childRow)
	}
}
