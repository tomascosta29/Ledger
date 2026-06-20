package commands_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/commands"
	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

func newTestDeps(t *testing.T) (commands.ImportDeps, func()) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.db")
	db, err := persistence.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	deps := commands.ImportDeps{
		TxRepo:    persistence.NewTransactionRepository(db),
		BatchRepo: persistence.NewImportBatchRepository(db),
		AuditRepo: persistence.NewAuditLogRepository(db),
		Now:       func() time.Time { return now },
	}
	return deps, func() { _ = db.Close() }
}

func writeTmpCSV(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func strPtr(s string) *string { return &s }

const ersteCSV = `"Own account name","Own IBAN","Booking Date","Partner Name","Partner IBAN","BIC/SWIFT","Partner Account Number","Bank code","Amount","Currency","Booking details","Booking Reference","Note","Highlights","Valuation Date"
"Giro","AT00XXX","30.04.2026","ACME","","","","20111","-30.90","EUR","Coffee","REF1","","0","30.04.2026"
"Giro","AT00XXX","30.04.2026","Beta","","","","20111","-1.00","EUR","Book","REF2","","0","30.04.2026"
"Giro","AT00XXX","01.05.2026","Salary Co","","","","20111","2500.00","EUR","May salary","REF3","","0","01.05.2026"
`

const revolutCSV = `Type,Product,Started Date,Completed Date,Description,Amount,Fee,Currency,State,Balance
Transfer,Savings,2026-03-27 09:21:45,2026-03-27 09:21:45,To pocket,1275.00,0.00,EUR,COMPLETED,1275.00
Card Payment,Current,2026-01-17 17:00:46,2026-01-17 17:05:00,Steam,-1.49,0.00,EUR,COMPLETED,100.00
Card Payment,Current,2026-01-17 17:00:46,,Steam,-1.49,0.00,EUR,REVERTED,101.49
`

func TestImportErsteDryRun(t *testing.T) {
	deps, cleanup := newTestDeps(t)
	defer cleanup()
	path := writeTmpCSV(t, "erste.csv", ersteCSV)

	uc := commands.NewImportUseCase(deps)
	res, err := uc.Execute(context.Background(), commands.ImportOptions{
		File: path, ProfileName: "erste", SourceFile: "erste.csv", DryRun: true,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Stats.RowsRead != 3 {
		t.Fatalf("rows read = %d, want 3", res.Stats.RowsRead)
	}
	if res.Stats.RowsSkipped != 0 || res.Stats.RowsInserted != 0 {
		t.Fatalf("dry-run should not change DB; got insert=%d skip=%d", res.Stats.RowsInserted, res.Stats.RowsSkipped)
	}
	if len(res.Preview) != 3 {
		t.Fatalf("preview = %d, want 3", len(res.Preview))
	}
}

func TestImportErsteRealRunAndDedup(t *testing.T) {
	deps, cleanup := newTestDeps(t)
	defer cleanup()
	path := writeTmpCSV(t, "erste.csv", ersteCSV)

	uc := commands.NewImportUseCase(deps)

	res, err := uc.Execute(context.Background(), commands.ImportOptions{
		File: path, ProfileName: "erste", SourceFile: "erste.csv",
	})
	if err != nil {
		t.Fatalf("first execute: %v", err)
	}
	if res.Stats.RowsInserted != 3 || res.Stats.RowsSkipped != 0 {
		t.Fatalf("first run: insert=%d skip=%d, want 3/0", res.Stats.RowsInserted, res.Stats.RowsSkipped)
	}

	res, err = uc.Execute(context.Background(), commands.ImportOptions{
		File: path, ProfileName: "erste", SourceFile: "erste.csv",
	})
	if err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if res.Stats.RowsInserted != 0 || res.Stats.RowsSkipped != 3 {
		t.Fatalf("second run: insert=%d skip=%d, want 0/3", res.Stats.RowsInserted, res.Stats.RowsSkipped)
	}

	count, err := deps.TxRepo.Count(context.Background(), ports.TxFilters{})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
}

func TestImportRevolutFiltersReverted(t *testing.T) {
	deps, cleanup := newTestDeps(t)
	defer cleanup()
	path := writeTmpCSV(t, "revolut.csv", revolutCSV)

	uc := commands.NewImportUseCase(deps)
	res, err := uc.Execute(context.Background(), commands.ImportOptions{
		File: path, ProfileName: "revolut", SourceFile: "revolut.csv",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Stats.RowsRead != 2 {
		t.Fatalf("rows read = %d, want 2 (REVERTED filtered)", res.Stats.RowsRead)
	}
	if res.Stats.RowsInserted != 2 {
		t.Fatalf("inserted = %d, want 2", res.Stats.RowsInserted)
	}
}

func TestImportAuditLogEntries(t *testing.T) {
	deps, cleanup := newTestDeps(t)
	defer cleanup()
	path := writeTmpCSV(t, "erste.csv", ersteCSV)

	uc := commands.NewImportUseCase(deps)
	res, err := uc.Execute(context.Background(), commands.ImportOptions{
		File: path, ProfileName: "erste", SourceFile: "erste.csv",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	entries, err := deps.AuditRepo.Query(context.Background(), ports.AuditEntryFilter{Action: strPtr(entities.AuditActionImport)})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != res.Stats.RowsInserted {
		t.Fatalf("audit entries = %d, want %d", len(entries), res.Stats.RowsInserted)
	}
}

func TestImportUnknownProfile(t *testing.T) {
	deps, cleanup := newTestDeps(t)
	defer cleanup()
	path := writeTmpCSV(t, "x.csv", ersteCSV)

	uc := commands.NewImportUseCase(deps)
	_, err := uc.Execute(context.Background(), commands.ImportOptions{
		File: path, ProfileName: "unicorn-bank",
	})
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}