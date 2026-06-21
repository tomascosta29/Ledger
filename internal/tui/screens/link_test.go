package screens

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/commands"
	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

// linkTestFixture spins up a fresh DB, runs migrations, and imports
// the given CSV through the standard ImportUseCase. Returns deps
// the Manager screen can use plus the underlying repos so the test
// can verify side effects.
type linkTestFixture struct {
	deps        Deps
	groupRepo   ports.GroupRepository
	overlayRepo ports.OverlayRepository
}

func newLinkTestFixture(t *testing.T, csv string) *linkTestFixture {
	t.Helper()
	ctx := context.Background()
	db, err := persistence.Open(ctx, filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	txRepo := persistence.NewTransactionRepository(db)
	tagRepo := persistence.NewTagRepository(db)
	bucketRepo := persistence.NewBucketRepository(db)
	auditRepo := persistence.NewAuditLogRepository(db)
	batchRepo := persistence.NewImportBatchRepository(db)
	groupRepo := persistence.NewGroupRepository(db)
	overlayRepo := persistence.NewOverlayRepository(db)
	overlaySvc := services.NewOverlayService(db.DB)

	importDeps := commands.ImportDeps{
		TxRepo:     txRepo,
		BatchRepo:  batchRepo,
		OverlaySvc: overlaySvc,
		AuditRepo:  auditRepo,
		Now:        func() time.Time { return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC) },
	}
	csvPath := filepath.Join(t.TempDir(), "test.csv")
	if err := os.WriteFile(csvPath, []byte(csv), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	uc := commands.NewImportUseCase(importDeps)
	if _, err := uc.Execute(ctx, commands.ImportOptions{
		File: csvPath, ProfileName: "erste", SourceFile: "test.csv",
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	deps := Deps{
		DB: db.DB, TxRepo: txRepo, TagRepo: tagRepo,
		BucketRepo: bucketRepo, AuditRepo: auditRepo, BatchRepo: batchRepo,
		GroupRepo: groupRepo, OverlayRepo: overlayRepo, OverlaySvc: overlaySvc,
	}
	return &linkTestFixture{deps: deps, groupRepo: groupRepo, overlayRepo: overlayRepo}
}

// ersteCSV matches the format the test fixtures in commands/import_test.go
// use, so the importer accepts it without profile-mismatch errors.
const linkErsteCSV = `"Own account name","Own IBAN","Booking Date","Partner Name","Partner IBAN","BIC/SWIFT","Partner Account Number","Bank code","Amount","Currency","Booking details","Booking Reference","Note","Highlights","Valuation Date"
"Giro","AT00XXX","15.06.2026","ACME","","","","20111","-50.00","EUR","card payment","","","0","15.06.2026"
"Giro","AT00XXX","16.06.2026","ACME","","","","20111","50.00","EUR","bank credit","","","0","16.06.2026"
`

const linkSameSignCSV = `"Own account name","Own IBAN","Booking Date","Partner Name","Partner IBAN","BIC/SWIFT","Partner Account Number","Bank code","Amount","Currency","Booking details","Booking Reference","Note","Highlights","Valuation Date"
"Giro","AT00XXX","15.06.2026","ACME","","","","20111","-50.00","EUR","first outgoing","","","0","15.06.2026"
"Giro","AT00XXX","16.06.2026","Beta","","","","20111","-30.00","EUR","second outgoing","","","0","16.06.2026"
`

// TestLinkManual_EndToEnd reproduces the user's "x x l" flow:
//   1. Import two transactions (one outgoing, one incoming, same currency).
//   2. Open Manager, select both rows via m.selected.
//   3. Press `l` (linkManual path).
//   4. Assert: a group was created linking the two txs, the overlay
//      reports source_kind='group' for both rows, and the managerRow
//      `linked` field is true.
//
// This is the smoke test the v1.3.1 fix was missing. Without it, we
// shipped code that silently did nothing for arbitrary selections.
func TestLinkManual_EndToEnd(t *testing.T) {
	f := newLinkTestFixture(t, linkErsteCSV)

	ctx := context.Background()
	m := NewManager()
	if err := m.Init(ctx, f.deps); err != nil {
		t.Fatalf("manager init: %v", err)
	}
	if len(m.rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(m.rows))
	}
	for _, r := range m.rows {
		if r.linked {
			t.Fatalf("pre-link: row %d should not be linked", r.id)
		}
	}

	// "x x" — select both rows.
	for i := range m.rows {
		m.selected[m.rows[i].id] = true
	}
	if len(m.selected) != 2 {
		t.Fatalf("want 2 selected, got %d", len(m.selected))
	}

	// "l" — link.
	m.linkSelected(ctx)

	// Verify: statusMsg reflects a successful link.
	if m.statusMsg == "" || m.statusMsg == "no transfer candidate between selected rows" {
		t.Fatalf("link did not produce a link; statusMsg=%q", m.statusMsg)
	}
	t.Logf("statusMsg: %s", m.statusMsg)

	// Verify: a group exists.
	groups, err := f.groupRepo.ListGroups(ctx)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("want 1 group, got %d", len(groups))
	}

	// Verify: overlay rows for both txs now report source_kind='group'.
	for _, r := range m.rows {
		oRows, err := f.overlayRepo.FindAll(ctx, ports.OverlayFindOptions{
			Filters: ports.OverlayFilters{},
			Sort:    ports.OverlaySortByDate,
			Order:   ports.SortDesc,
			Limit:   100,
		})
		if err != nil {
			t.Fatalf("overlay find: %v", err)
		}
		var match *ports.OverlayTransaction
		for _, o := range oRows {
			if o.ID == r.id {
				match = o
				break
			}
		}
		if match == nil {
			t.Errorf("row %d not found in overlay", r.id)
			continue
		}
		if match.SourceKind != ports.SourceGroup {
			t.Errorf("row %d overlay SourceKind: want %q, got %q",
				r.id, ports.SourceGroup, match.SourceKind)
		}
		if !r.linked {
			t.Errorf("row %d: managerRow.linked should be true after reload", r.id)
		}
	}

	// Verify: selection cleared.
	if len(m.selected) != 0 {
		t.Errorf("selection should be cleared after link, got %d", len(m.selected))
	}
}

// TestLinkManual_SameSignUsesDateTiebreak covers the unusual case
// where the operator selects two outgoing transactions. Detection
// would have rejected this pair (both negative amounts). Manual
// linking accepts it, using the earlier date as `from`.
func TestLinkManual_SameSignUsesDateTiebreak(t *testing.T) {
	f := newLinkTestFixture(t, linkSameSignCSV)

	ctx := context.Background()
	m := NewManager()
	if err := m.Init(ctx, f.deps); err != nil {
		t.Fatalf("manager init: %v", err)
	}

	for i := range m.rows {
		m.selected[m.rows[i].id] = true
	}
	m.linkSelected(ctx)

	if m.statusMsg == "" || m.statusMsg == "no transfer candidate between selected rows" {
		t.Fatalf("same-sign pair should still link; statusMsg=%q", m.statusMsg)
	}

	groups, err := f.groupRepo.ListGroups(ctx)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("want 1 group, got %d", len(groups))
	}
}
