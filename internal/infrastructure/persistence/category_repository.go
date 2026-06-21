package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
	return r.GetByIDDBTX(ctx, r.db, id)
}

func (r *CategoryRepository) GetByIDDBTX(ctx context.Context, db ports.DBTX, id int64) (*entities.Category, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, name, description, archived_at, created_at
		 FROM categories WHERE id = ?`, id)
	return scanCategory(row)
}

func (r *CategoryRepository) GetByName(ctx context.Context, name string) (*entities.Category, error) {
	return r.GetByNameDBTX(ctx, r.db, name)
}

func (r *CategoryRepository) GetByNameDBTX(ctx context.Context, db ports.DBTX, name string) (*entities.Category, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, name, description, archived_at, created_at
		 FROM categories WHERE name = ?`, name)
	return scanCategory(row)
}

func (r *CategoryRepository) Create(ctx context.Context, c *entities.Category) (int64, error) {
	return r.CreateDBTX(ctx, r.db, c)
}

func (r *CategoryRepository) CreateDBTX(ctx context.Context, db ports.DBTX, c *entities.Category) (int64, error) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := db.ExecContext(ctx,
		`INSERT INTO categories (name, description, archived_at, created_at) VALUES (?, ?, ?, ?)`,
		c.Name, c.Description, nullStr(toPtrStr(c.ArchivedAt)), now,
	)
	if err != nil {
		return 0, fmt.Errorf("create category: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

func (r *CategoryRepository) Rename(ctx context.Context, id int64, newName string) error {
	return r.RenameDBTX(ctx, r.db, id, newName)
}

func (r *CategoryRepository) RenameDBTX(ctx context.Context, db ports.DBTX, id int64, newName string) error {
	res, err := db.ExecContext(ctx,
		`UPDATE categories SET name = ? WHERE id = ?`,
		newName, id,
	)
	if err != nil {
		return fmt.Errorf("rename category: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (r *CategoryRepository) Archive(ctx context.Context, id int64) error {
	return r.ArchiveDBTX(ctx, r.db, id)
}

func (r *CategoryRepository) ArchiveDBTX(ctx context.Context, db ports.DBTX, id int64) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := db.ExecContext(ctx,
		`UPDATE categories SET archived_at = ? WHERE id = ? AND archived_at IS NULL`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("archive category: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func toPtrStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format("2006-01-02T15:04:05.000Z")
	return &s
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
