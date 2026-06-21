package commands_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/commands"
	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

func newCmdTestDB(t *testing.T) *persistence.DB {
	t.Helper()
	db, err := persistence.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestManualAddCreatesAndIsIdempotent(t *testing.T) {
	db := newCmdTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO categories (name) VALUES ('want')`); err != nil {
		t.Fatalf("seed want: %v", err)
	}

	uc := commands.NewManualAddUseCase(commands.ManualAddDeps{
		TxRepo:       persistence.NewTransactionRepository(db),
		AuditRepo:    persistence.NewAuditLogRepository(db),
		CategoryRepo: persistence.NewCategoryRepository(db),
		OverlaySvc:   services.NewOverlayService(db.DB),
		Now:          func() time.Time { return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC) },
	})

	opts := commands.ManualAddOptions{
		EffectiveDate: "2026-06-15",
		Amount:        "-25.50",
		Currency:      "EUR",
		Description:   "Coffee with team",
		PartnerName:   "Café Test",
		Category:      "want",
	}
	first, err := uc.Execute(context.Background(), opts)
	if err != nil {
		t.Fatalf("first execute: %v", err)
	}
	if !first.Created {
		t.Fatal("expected Created=true on first call")
	}

	second, err := uc.Execute(context.Background(), opts)
	if err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if second.Created {
		t.Fatal("expected Created=false on duplicate")
	}
	if second.TransactionID != first.TransactionID {
		t.Fatalf("expected same id, got %d vs %d", second.TransactionID, first.TransactionID)
	}

	// Audit log should have exactly one import entry for this tx.
	auditRepo := persistence.NewAuditLogRepository(db)
	entries, err := auditRepo.Query(context.Background(), ports.AuditEntryFilter{RecordID: &first.TransactionID})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	var imports int
	for _, e := range entries {
		if e.Action == "import" {
			imports++
		}
	}
	if imports != 1 {
		t.Fatalf("expected 1 import audit row, got %d", imports)
	}
}

func TestManualAddRequiresFields(t *testing.T) {
	db := newCmdTestDB(t)
	defer db.Close()

	uc := commands.NewManualAddUseCase(commands.ManualAddDeps{
		TxRepo:       persistence.NewTransactionRepository(db),
		AuditRepo:    persistence.NewAuditLogRepository(db),
		CategoryRepo: persistence.NewCategoryRepository(db),
		OverlaySvc:   services.NewOverlayService(db.DB),
	})
	cases := []struct {
		name string
		opts commands.ManualAddOptions
		want string
	}{
		{"missing date", commands.ManualAddOptions{Currency: "EUR", Amount: "1", Description: "x"}, "--date"},
		{"missing currency", commands.ManualAddOptions{EffectiveDate: "2026-06-15", Amount: "1", Description: "x"}, "--currency"},
		{"missing description", commands.ManualAddOptions{EffectiveDate: "2026-06-15", Amount: "1", Currency: "EUR"}, "--description"},
		{"bad amount", commands.ManualAddOptions{EffectiveDate: "2026-06-15", Amount: "abc", Currency: "EUR", Description: "x"}, "amount"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := uc.Execute(context.Background(), c.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
