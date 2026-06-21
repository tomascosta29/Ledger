package services_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

func newSummaryTestEnv(t *testing.T) (*services.SummaryService, *persistence.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.Open(context.Background(), filepath.Join(dir, "ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.Migrate(db.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Seed categories referenced by the test transactions.
	if _, err := db.Exec(`INSERT INTO categories (name) VALUES ('need'), ('want'), ('savings')`); err != nil {
		t.Fatalf("seed categories: %v", err)
	}
	categoryRepo := persistence.NewCategoryRepository(db)
	catID := map[string]int64{}
	for _, name := range []string{"need", "want", "savings"} {
		c, err := categoryRepo.GetByName(context.Background(), name)
		if err != nil {
			t.Fatalf("lookup seeded %q: %v", name, err)
		}
		catID[name] = c.ID
	}
	// Seed transactions across two months.
	txRepo := persistence.NewTransactionRepository(db)
	for i, e := range []struct {
		date   string
		amount int64
		cat    string
	}{
		{"2026-06-01", -1000, "need"},
		{"2026-06-15", -500, "want"},
		{"2026-06-20", 2500, "savings"},
		{"2026-07-01", -2000, "need"},
	} {
		cid := catID[e.cat]
		id, err := txRepo.Insert(context.Background(), &entities.Transaction{
			EffectiveDate: e.date,
			Amount:        valueobjects.MustNew(e.amount, valueobjects.EUR),
			Description:   "tx",
			SourceHash:    "h" + itoa(i),
			CategoryID:    &cid,
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		_ = id
	}
	// Rebuild overlay so the summary's query sees the seeded transactions.
	if err := services.NewOverlayService(db.DB).Rebuild(context.Background()); err != nil {
		t.Fatalf("rebuild overlay: %v", err)
	}

	// Set up recipes in a temp dir.
	recipesDir := filepath.Join(dir, "recipes")
	t.Setenv("LEDGER_RECIPES_DIR", recipesDir)
	if err := writeRecipe(recipesDir, "essentials", entities.Recipe{
		Name:        "essentials",
		Description: "need + want",
		Include: []entities.Clause{
			{Field: "category", Op: "is", Value: "need"},
			{Field: "category", Op: "is", Value: "want"},
		},
	}); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
	svc := services.NewSummaryService(services.SummaryDeps{
		OverlayRepo: persistence.NewOverlayRepository(db),
		RecipeRepo:  persistence.NewRecipeRepository(db),
		TagRepo:     persistence.NewTagRepository(db),
	})
	return svc, db, func() { _ = db.Close() }
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var b []byte
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		b = append([]byte{digits[i%10]}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func writeRecipe(dir, name string, r entities.Recipe) error {
	body := "name = \"" + r.Name + "\"\n"
	if r.Description != "" {
		body += "description = \"" + r.Description + "\"\n"
	}
	body += "include = [\n"
	for _, c := range r.Include {
		body += "  { field = \"" + c.Field + "\", op = \"" + c.Op + "\", value = \"" + c.Value + "\" },\n"
	}
	body += "]\nexclude = []\nnet = false\n"
	return writeFile(filepath.Join(dir, name+".toml"), body)
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestSummaryFiltersByCategory(t *testing.T) {
	svc, _, cleanup := newSummaryTestEnv(t)
	defer cleanup()
	result, err := svc.Run(context.Background(), "essentials", "2026-06")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.RecipeName != "essentials" {
		t.Errorf("recipe = %q", result.RecipeName)
	}
	if len(result.Lines) != 1 {
		t.Fatalf("expected 1 currency, got %d", len(result.Lines))
	}
	if result.Lines[0].Currency != valueobjects.EUR {
		t.Errorf("currency = %s", result.Lines[0].Currency)
	}
	if result.Lines[0].Expense != -1500 {
		t.Errorf("expense = %d, want -1500", result.Lines[0].Expense)
	}
	if result.Lines[0].Income != 0 {
		t.Errorf("income = %d, want 0 (savings excluded)", result.Lines[0].Income)
	}
	if result.Lines[0].Count != 2 {
		t.Errorf("count = %d, want 2", result.Lines[0].Count)
	}
}

func TestSummaryActiveRecipeFallback(t *testing.T) {
	svc, db, cleanup := newSummaryTestEnv(t)
	defer cleanup()
	repo := persistence.NewRecipeRepository(db)
	if err := repo.SetActiveName(context.Background(), "essentials"); err != nil {
		t.Fatalf("set active: %v", err)
	}
	result, err := svc.Run(context.Background(), "", "2026-07")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.RecipeName != "essentials" {
		t.Errorf("expected active recipe, got %q", result.RecipeName)
	}
	if len(result.Lines) != 1 || result.Lines[0].Count != 1 {
		t.Fatalf("expected 1 tx in 2026-07 matching essentials, got %+v", result.Lines)
	}
}

func TestSummaryUnknownRecipe(t *testing.T) {
	svc, _, cleanup := newSummaryTestEnv(t)
	defer cleanup()
	_, err := svc.Run(context.Background(), "nonexistent", "2026-06")
	if err == nil {
		t.Fatal("expected error for unknown recipe")
	}
}
