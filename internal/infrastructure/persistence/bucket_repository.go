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

type BucketRepository struct {
	db *sql.DB
}

func NewBucketRepository(db *DB) *BucketRepository {
	return &BucketRepository{db: db.DB}
}

func (r *BucketRepository) Create(ctx context.Context, b *entities.Bucket) (int64, error) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO buckets (name, currency, monthly_allocation_minor, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		b.Name, b.Currency, b.MonthlyAllocationMinor, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create bucket: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

func (r *BucketRepository) GetByID(ctx context.Context, id int64) (*entities.Bucket, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, currency, monthly_allocation_minor, archived_at, created_at, updated_at
		 FROM buckets WHERE id = ?`, id)
	return scanBucket(row)
}

func (r *BucketRepository) GetByName(ctx context.Context, name string) (*entities.Bucket, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, currency, monthly_allocation_minor, archived_at, created_at, updated_at
		 FROM buckets WHERE name = ?`, name)
	return scanBucket(row)
}

func (r *BucketRepository) List(ctx context.Context, includeArchived bool) ([]*entities.Bucket, error) {
	q := `SELECT id, name, currency, monthly_allocation_minor, archived_at, created_at, updated_at
	      FROM buckets`
	if !includeArchived {
		q += ` WHERE archived_at IS NULL`
	}
	q += ` ORDER BY name`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	defer rows.Close()
	out := make([]*entities.Bucket, 0, 8)
	for rows.Next() {
		b, err := scanBucket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *BucketRepository) Update(ctx context.Context, b *entities.Bucket) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := r.db.ExecContext(ctx,
		`UPDATE buckets SET name = ?, currency = ?, monthly_allocation_minor = ?, updated_at = ?
		 WHERE id = ?`,
		b.Name, b.Currency, b.MonthlyAllocationMinor, now, b.ID,
	)
	if err != nil {
		return fmt.Errorf("update bucket: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (r *BucketRepository) Archive(ctx context.Context, id int64) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := r.db.ExecContext(ctx,
		`UPDATE buckets SET archived_at = ?, updated_at = ? WHERE id = ? AND archived_at IS NULL`,
		now, now, id,
	)
	if err != nil {
		return fmt.Errorf("archive bucket: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (r *BucketRepository) Delete(ctx context.Context, id int64) error {
	count, err := r.CountAssignedTransactions(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("bucket has %d assigned transaction(s); archive instead", count)
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM buckets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete bucket: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (r *BucketRepository) CountAssignedTransactions(ctx context.Context, id int64) (int64, error) {
	var n int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transactions WHERE bucket_id = ?`, id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count assigned: %w", err)
	}
	return n, nil
}

type BucketSpend = ports.BucketSpend

// SpendByMonth returns per-bucket spend for transactions in the given
// month (YYYY-MM), excluding hidden ones. SpentMinor is the absolute
// value of the sum of negative (expense) amounts in that month — i.e.
// the "money out of the bucket" figure a budget cares about. month is
// the calendar month as a prefix match against effective_date.
func (r *BucketRepository) SpendByMonth(ctx context.Context, month string) ([]ports.BucketSpend, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			b.id, b.name, b.currency, b.monthly_allocation_minor,
			COALESCE(-SUM(CASE WHEN t.amount_minor < 0 THEN t.amount_minor ELSE 0 END), 0) AS spent,
			COUNT(t.id) AS cnt
		FROM buckets b
		LEFT JOIN transactions t
		  ON t.bucket_id = b.id
		 AND substr(t.effective_date, 1, 7) = ?
		 AND t.is_hidden = 0
		WHERE b.archived_at IS NULL
		GROUP BY b.id
		ORDER BY b.name
	`, month)
	if err != nil {
		return nil, fmt.Errorf("spend by month: %w", err)
	}
	defer rows.Close()
	out := make([]ports.BucketSpend, 0, 8)
	for rows.Next() {
		var s ports.BucketSpend
		if err := rows.Scan(&s.BucketID, &s.BucketName, &s.Currency, &s.AllocatedMinor, &s.SpentMinor, &s.Count); err != nil {
			return nil, fmt.Errorf("scan bucket spend: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// UnassignedSpendByMonth returns the total expense (negative amounts,
// absolute value) for the given month that has no bucket assigned,
// grouped by currency.
func (r *BucketRepository) UnassignedSpendByMonth(ctx context.Context, month string) ([]ports.BucketSpend, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			0 AS id, '(unassigned)' AS name, t.currency, 0 AS allocated,
			COALESCE(-SUM(CASE WHEN t.amount_minor < 0 THEN t.amount_minor ELSE 0 END), 0) AS spent,
			COUNT(t.id) AS cnt
		FROM transactions t
		WHERE t.bucket_id IS NULL
		  AND substr(t.effective_date, 1, 7) = ?
		  AND t.is_hidden = 0
		GROUP BY t.currency
		ORDER BY t.currency
	`, month)
	if err != nil {
		return nil, fmt.Errorf("unassigned spend: %w", err)
	}
	defer rows.Close()
	out := make([]ports.BucketSpend, 0, 2)
	for rows.Next() {
		var s ports.BucketSpend
		if err := rows.Scan(&s.BucketID, &s.BucketName, &s.Currency, &s.AllocatedMinor, &s.SpentMinor, &s.Count); err != nil {
			return nil, fmt.Errorf("scan unassigned: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type bucketScanner interface {
	Scan(dest ...any) error
}

func scanBucket(s bucketScanner) (*entities.Bucket, error) {
	var (
		b          entities.Bucket
		archivedAt sql.NullString
		createdAt  string
		updatedAt  string
	)
	err := s.Scan(&b.ID, &b.Name, &b.Currency, &b.MonthlyAllocationMinor, &archivedAt, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, fmt.Errorf("scan bucket: %w", err)
	}
	if archivedAt.Valid {
		t := parseISO(archivedAt.String)
		b.ArchivedAt = &t
	}
	b.CreatedAt = parseISO(createdAt)
	b.UpdatedAt = parseISO(updatedAt)
	return &b, nil
}
