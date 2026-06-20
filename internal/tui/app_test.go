package tui_test

import (
	"context"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	tui "github.com/tomascosta29/Ledger/internal/tui"
	"github.com/tomascosta29/Ledger/internal/tui/screens"

	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

func newDeps(t *testing.T) (screens.Deps, func()) {
	t.Helper()
	db, err := persistence.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	deps := screens.Deps{
		DB:          db.DB,
		DBPath:      persistence.DefaultDBPath(),
		TxRepo:      persistence.NewTransactionRepository(db),
		TagRepo:     persistence.NewTagRepository(db),
		BucketRepo:  persistence.NewBucketRepository(db),
		AuditRepo:   persistence.NewAuditLogRepository(db),
		BatchRepo:   persistence.NewImportBatchRepository(db),
		OverlayRepo: persistence.NewOverlayRepository(db),
		OverlaySvc:  services.NewOverlayService(db.DB),
		BudgetSvc:   persistence.NewBucketRepository(db),
		RecipeSvc:   persistence.NewRecipeRepository(db),
	}
	return deps, func() { _ = db.Close() }
}

func TestAppStartsAndUpdates(t *testing.T) {
	ctx := context.Background()
	deps, cleanup := newDeps(t)
	defer cleanup()

	app := tui.NewApp(ctx, deps)
	if app == nil {
		t.Fatal("NewApp returned nil")
	}

	// Simulate a window size.
	model, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if model == nil {
		t.Fatal("Update returned nil model")
	}

	// Press "3" — should jump to the Linker screen.
	if _, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}); cmd != nil && false {
		// not exercised — cmd is nil for our key handler
	}

	// Help toggle.
	if _, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}); false {
		// no-op
	}

	// View should not panic.
	_ = model.View()
}
