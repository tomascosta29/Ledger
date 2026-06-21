package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type CategoryRepository struct {
	db *sql.DB
}

func NewCategoryRepository(db *DB) *CategoryRepository {
	return &CategoryRepository{db: db.DB}
}

func (r *CategoryRepository) List(ctx context.Context, includeArchived bool) ([]*entities.Category, error) {
	q := `SELECT id, name, description, archived_at, created_at
	      FROM categories`
	if !includeArchived {
		q += ` WHERE archived_at IS NULL`
	}
	q += ` ORDER BY name`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()
	out := make([]*entities.Category, 0, 8)
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CategoryRepository) GetByID(ctx context.Context, id int64) (*entities.Category, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, archived_at, created_at
		 FROM categories WHERE id = ?`, id)
	return scanCategory(row)
}

func (r *CategoryRepository) GetByName(ctx context.Context, name string) (*entities.Category, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, archived_at, created_at
		 FROM categories WHERE name = ?`, name)
	return scanCategory(row)
}

type categoryScanner interface {
	Scan(dest ...any) error
}

func scanCategory(s categoryScanner) (*entities.Category, error) {
	var (
		c          entities.Category
		archivedAt sql.NullString
		createdAt  string
	)
	err := s.Scan(&c.ID, &c.Name, &c.Description, &archivedAt, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, fmt.Errorf("scan category: %w", err)
	}
	if archivedAt.Valid {
		t := parseISO(archivedAt.String)
		c.ArchivedAt = &t
	}
	c.CreatedAt = parseISO(createdAt)
	return &c, nil
}
