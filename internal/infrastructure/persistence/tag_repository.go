package persistence

import (
	"context"
	"fmt"
	"strings"

	"github.com/tomascosta29/Ledger/internal/application/ports"
)

type TagRepository struct {
	db *DB
}

func NewTagRepository(db *DB) *TagRepository {
	return &TagRepository{db: db}
}

func (r *TagRepository) Add(ctx context.Context, transactionID int64, tag string) error {
	return r.AddDBTX(ctx, r.db, transactionID, tag)
}

func (r *TagRepository) AddDBTX(ctx context.Context, db ports.DBTX, transactionID int64, tag string) error {
	tag = normalizeTag(tag)
	if tag == "" {
		return fmt.Errorf("tag is empty")
	}
	_, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO transaction_tags (transaction_id, tag) VALUES (?, ?)
	`, transactionID, tag)
	if err != nil {
		return fmt.Errorf("add tag: %w", err)
	}
	return nil
}

func (r *TagRepository) Remove(ctx context.Context, transactionID int64, tag string) error {
	return r.RemoveDBTX(ctx, r.db, transactionID, tag)
}

func (r *TagRepository) RemoveDBTX(ctx context.Context, db ports.DBTX, transactionID int64, tag string) error {
	tag = normalizeTag(tag)
	if tag == "" {
		return fmt.Errorf("tag is empty")
	}
	_, err := db.ExecContext(ctx, `DELETE FROM transaction_tags WHERE transaction_id = ? AND tag = ?`, transactionID, tag)
	if err != nil {
		return fmt.Errorf("remove tag: %w", err)
	}
	return nil
}

var _ ports.DBTX = (*DB)(nil)

func (r *TagRepository) ListByTransaction(ctx context.Context, transactionID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT tag FROM transaction_tags WHERE transaction_id = ? ORDER BY tag
	`, transactionID)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *TagRepository) ListByTag(ctx context.Context, tag string) ([]int64, error) {
	tag = normalizeTag(tag)
	rows, err := r.db.QueryContext(ctx, `
		SELECT transaction_id FROM transaction_tags WHERE tag = ? ORDER BY transaction_id
	`, tag)
	if err != nil {
		return nil, fmt.Errorf("list by tag: %w", err)
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func normalizeTag(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func (r *TagRepository) Clear(ctx context.Context, transactionID int64) error {
	return r.ClearDBTX(ctx, r.db, transactionID)
}

func (r *TagRepository) ClearDBTX(ctx context.Context, db ports.DBTX, transactionID int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM transaction_tags WHERE transaction_id = ?`, transactionID)
	if err != nil {
		return fmt.Errorf("clear tags: %w", err)
	}
	return nil
}
