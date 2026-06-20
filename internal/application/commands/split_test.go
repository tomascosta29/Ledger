package commands_test

import (
	"context"
	"testing"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/commands"
	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

func seedTx(t *testing.T, db *persistence.DB, amount int64) int64 {
	t.Helper()
	id, err := persistence.NewTransactionRepository(db).Insert(context.Background(), &entities.Transaction{
		EffectiveDate: "2026-06-20",
		Amount:        valueobjects.MustNew(amount, valueobjects.EUR),
		Description:   "seed",
		SourceHash:    "seed-hash",
		Category:      "Unknown",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return id
}

func TestSplitSucceeds(t *testing.T) {
	db := newCmdTestDB(t)
	defer db.Close()
	parentID := seedTx(t, db, -10000)

	uc := commands.NewSplitUseCase(commands.SplitDeps{
		TxRepo:     persistence.NewTransactionRepository(db),
		AuditRepo:  persistence.NewAuditLogRepository(db),
		OverlaySvc: services.NewOverlayService(db.DB),
		Now:        func() time.Time { return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC) },
	})
	res, err := uc.Execute(context.Background(), commands.SplitOptions{
		TransactionID: parentID,
		Children: []commands.SplitChild{
			{AmountMinor: -6000, Currency: valueobjects.EUR, Description: "groceries"},
			{AmountMinor: -4000, Currency: valueobjects.EUR, Description: "household"},
		},
	})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if res.ParentID != parentID || len(res.ChildrenIDs) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}

	children, err := persistence.NewTransactionRepository(db).FindByParent(context.Background(), parentID)
	if err != nil {
		t.Fatalf("find children: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}

	// Overlay should have 1 split_header + 2 split_child, no raw row for the parent.
	rows, err := persistence.NewOverlayRepository(db).FindAll(context.Background(), ports.OverlayFindOptions{})
	if err != nil {
		t.Fatalf("find overlay: %v", err)
	}
	var header, child int
	for _, r := range rows {
		switch r.SourceKind {
		case ports.SourceSplitHeader:
			header++
		case ports.SourceSplitChild:
			child++
		}
	}
	if header != 1 {
		t.Errorf("expected 1 split_header, got %d", header)
	}
	if child != 2 {
		t.Errorf("expected 2 split_child, got %d", child)
	}
}

func TestSplitRejectsNonMatchingSum(t *testing.T) {
	db := newCmdTestDB(t)
	defer db.Close()
	parentID := seedTx(t, db, -10000)

	uc := commands.NewSplitUseCase(commands.SplitDeps{
		TxRepo:     persistence.NewTransactionRepository(db),
		AuditRepo:  persistence.NewAuditLogRepository(db),
		OverlaySvc: services.NewOverlayService(db.DB),
	})
	_, err := uc.Execute(context.Background(), commands.SplitOptions{
		TransactionID: parentID,
		Children: []commands.SplitChild{
			{AmountMinor: -5000, Currency: valueobjects.EUR, Description: "half"},
			{AmountMinor: -4000, Currency: valueobjects.EUR, Description: "less"},
		},
	})
	if err == nil {
		t.Fatal("expected sum mismatch error")
	}
}

func TestSplitRejectsReSplit(t *testing.T) {
	db := newCmdTestDB(t)
	defer db.Close()
	parentID := seedTx(t, db, -10000)

	uc := commands.NewSplitUseCase(commands.SplitDeps{
		TxRepo:     persistence.NewTransactionRepository(db),
		AuditRepo:  persistence.NewAuditLogRepository(db),
		OverlaySvc: services.NewOverlayService(db.DB),
	})
	_, err := uc.Execute(context.Background(), commands.SplitOptions{
		TransactionID: parentID,
		Children: []commands.SplitChild{
			{AmountMinor: -6000, Currency: valueobjects.EUR, Description: "a"},
			{AmountMinor: -4000, Currency: valueobjects.EUR, Description: "b"},
		},
	})
	if err != nil {
		t.Fatalf("first split: %v", err)
	}
	_, err = uc.Execute(context.Background(), commands.SplitOptions{
		TransactionID: parentID,
		Children: []commands.SplitChild{
			{AmountMinor: -3000, Currency: valueobjects.EUR, Description: "c"},
			{AmountMinor: -7000, Currency: valueobjects.EUR, Description: "d"},
		},
	})
	if err == nil {
		t.Fatal("expected re-split error")
	}
}
