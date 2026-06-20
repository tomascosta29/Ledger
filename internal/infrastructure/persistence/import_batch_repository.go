package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type ImportBatchRepository struct {
	db *DB
}

func NewImportBatchRepository(db *DB) *ImportBatchRepository {
	return &ImportBatchRepository{db: db}
}

func (r *ImportBatchRepository) Create(ctx context.Context, batch *entities.ImportBatch) (int64, error) {
	now := batch.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO import_batches (source_file, source_profile, row_count, inserted_count, skipped_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		batch.SourceFile, batch.SourceProfile, batch.RowCount, batch.InsertedCount, batch.SkippedCount,
		timeToISO(now),
	)
	if err != nil {
		return 0, fmt.Errorf("insert batch: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	batch.ID = id
	batch.CreatedAt = now
	return id, nil
}

func (r *ImportBatchRepository) UpdateCounts(ctx context.Context, id int64, inserted, skipped int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE import_batches SET inserted_count = ?, skipped_count = ? WHERE id = ?
	`, inserted, skipped, id)
	if err != nil {
		return fmt.Errorf("update batch counts: %w", err)
	}
	return nil
}

func (r *ImportBatchRepository) GetByID(ctx context.Context, id int64) (*entities.ImportBatch, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, source_file, source_profile, row_count, inserted_count, skipped_count, created_at
		FROM import_batches WHERE id = ?
	`, id)
	return scanBatch(row)
}

func (r *ImportBatchRepository) Recent(ctx context.Context, limit int) ([]*entities.ImportBatch, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, source_file, source_profile, row_count, inserted_count, skipped_count, created_at
		FROM import_batches ORDER BY created_at DESC, id DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent batches: %w", err)
	}
	defer rows.Close()
	out := make([]*entities.ImportBatch, 0, limit)
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func scanBatch(s scanner) (*entities.ImportBatch, error) {
	var (
		b         entities.ImportBatch
		createdAt string
	)
	err := s.Scan(&b.ID, &b.SourceFile, &b.SourceProfile, &b.RowCount, &b.InsertedCount, &b.SkippedCount, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan batch: %w", err)
	}
	b.CreatedAt = parseISO(createdAt)
	return &b, nil
}