package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type RecipeRepository struct {
	dir string
	db  *sql.DB
}

func NewRecipeRepository(db *DB) *RecipeRepository {
	dir := os.Getenv("LEDGER_RECIPES_DIR")
	if dir == "" {
		dir = defaultRecipesDir()
	}
	return &RecipeRepository{dir: dir, db: db.DB}
}

func defaultRecipesDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ledger", "recipes")
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	return filepath.Join(home, ".config", "ledger", "recipes")
}

func (r *RecipeRepository) LoadAll(ctx context.Context) ([]*entities.Recipe, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read recipes dir: %w", err)
	}
	out := make([]*entities.Recipe, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		rec, err := r.loadFile(filepath.Join(r.dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", e.Name(), err)
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *RecipeRepository) LoadByName(ctx context.Context, name string) (*entities.Recipe, error) {
	all, err := r.LoadAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, rec := range all {
		if rec.Name == name {
			return rec, nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *RecipeRepository) loadFile(path string) (*entities.Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rec entities.Recipe
	if _, err := toml.Decode(string(data), &rec); err != nil {
		return nil, err
	}
	if rec.Name == "" {
		// default to filename without extension
		base := filepath.Base(path)
		rec.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return &rec, nil
}

func (r *RecipeRepository) GetActiveName(ctx context.Context) (string, error) {
	var v string
	err := r.db.QueryRowContext(ctx,
		`SELECT value FROM recipes_state WHERE key = 'active'`).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("get active recipe: %w", err)
	}
	return v, nil
}

func (r *RecipeRepository) SetActiveName(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO recipes_state (key, value) VALUES ('active', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, name)
	if err != nil {
		return fmt.Errorf("set active recipe: %w", err)
	}
	return nil
}
