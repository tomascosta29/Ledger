package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
)

var ErrNotFound = errors.New("not found")

type TransactionRepository struct {
	db *DB
}

func NewTransactionRepository(db *DB) *TransactionRepository {
	return &TransactionRepository{db: db}
}

func (r *TransactionRepository) Insert(ctx context.Context, tx *entities.Transaction) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO transactions (
			effective_date, amount_minor, currency, description,
			partner_name, partner_iban, import_batch_id, source_hash,
			raw_data, raw_description, category,
			exclude_from_reports, is_hidden, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		tx.EffectiveDate,
		tx.Amount.Amount,
		string(tx.Amount.Currency),
		tx.Description,
		nullStr(tx.PartnerName),
		nullStr(tx.PartnerIBAN),
		nullInt64(tx.ImportBatchID),
		tx.SourceHash,
		nullBytes(tx.RawData),
		nullStr(tx.RawDescription),
		tx.Category,
		boolToInt(tx.ExcludeFromReports),
		boolToInt(tx.IsHidden),
		timeToISO(tx.CreatedAt),
		timeToISO(tx.UpdatedAt),
	)
	if err != nil {
		return 0, fmt.Errorf("insert transaction: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	tx.ID = id
	return id, nil
}

func (r *TransactionRepository) InsertBatch(ctx context.Context, txs []*entities.Transaction) ([]int64, error) {
	if len(txs) == 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO transactions (
			effective_date, amount_minor, currency, description,
			partner_name, partner_iban, import_batch_id, source_hash,
			raw_data, raw_description, category,
			exclude_from_reports, is_hidden, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	ids := make([]int64, 0, len(txs))
	for _, t := range txs {
		res, err := stmt.ExecContext(ctx,
			t.EffectiveDate,
			t.Amount.Amount,
			string(t.Amount.Currency),
			t.Description,
			nullStr(t.PartnerName),
			nullStr(t.PartnerIBAN),
			nullInt64(t.ImportBatchID),
			t.SourceHash,
			nullBytes(t.RawData),
			nullStr(t.RawDescription),
			t.Category,
			boolToInt(t.ExcludeFromReports),
			boolToInt(t.IsHidden),
			timeToISO(t.CreatedAt),
			timeToISO(t.UpdatedAt),
		)
		if err != nil {
			return nil, fmt.Errorf("exec insert: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("last insert id: %w", err)
		}
		t.ID = id
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return ids, nil
}

func (r *TransactionRepository) GetByID(ctx context.Context, id int64) (*entities.Transaction, error) {
	row := r.db.QueryRowContext(ctx, selectAllColumnsSQL+" WHERE id = ?", id)
	return scanTransaction(row)
}

func (r *TransactionRepository) GetBySourceHash(ctx context.Context, hash string) (*entities.Transaction, error) {
	row := r.db.QueryRowContext(ctx, selectAllColumnsSQL+" WHERE source_hash = ?", hash)
	return scanTransaction(row)
}

func (r *TransactionRepository) FindAll(ctx context.Context, opts ports.TxFindOptions) ([]*entities.Transaction, error) {
	where, args := buildWhere(opts.Filters)
	q := selectAllColumnsSQL + where
	if opts.Sort != "" {
		sortCol := string(opts.Sort)
		if !allowedSortColumns[sortCol] {
			return nil, fmt.Errorf("invalid sort column: %s", sortCol)
		}
		order := "ASC"
		if opts.Order == ports.SortDesc {
			order = "DESC"
		}
		q += fmt.Sprintf(" ORDER BY %s %s", sortCol, order)
	} else {
		q += " ORDER BY effective_date DESC"
	}
	if opts.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query transactions: %w", err)
	}
	defer rows.Close()

	out := make([]*entities.Transaction, 0, 32)
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *TransactionRepository) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	setClauses := make([]string, 0, len(fields)+1)
	args := make([]any, 0, len(fields)+2)
	for k, v := range fields {
		if !allowedUpdateColumns[k] {
			return fmt.Errorf("invalid update column: %s", k)
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", k))
		switch val := v.(type) {
		case *string:
			args = append(args, nullStr(val))
		case *int64:
			args = append(args, nullInt64(val))
		case bool:
			args = append(args, boolToInt(val))
		case string:
			args = append(args, val)
		case int64:
			args = append(args, val)
		case int:
			args = append(args, val)
		default:
			args = append(args, v)
		}
	}
	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, timeToISO(time.Now().UTC()))
	args = append(args, id)
	q := "UPDATE transactions SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update transaction %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *TransactionRepository) SetHidden(ctx context.Context, id int64, hidden bool) error {
	return r.UpdateFields(ctx, id, map[string]any{"is_hidden": hidden})
}

func (r *TransactionRepository) SetExcludeFromReports(ctx context.Context, id int64, exclude bool) error {
	return r.UpdateFields(ctx, id, map[string]any{"exclude_from_reports": exclude})
}

func (r *TransactionRepository) SetCategory(ctx context.Context, id int64, category string) error {
	return r.UpdateFields(ctx, id, map[string]any{"category": category})
}

func (r *TransactionRepository) Count(ctx context.Context, filters ports.TxFilters) (int64, error) {
	where, args := buildWhere(filters)
	q := "SELECT COUNT(*) FROM transactions" + where
	var n int64
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	return n, nil
}

const selectAllColumnsSQL = `SELECT
	id, effective_date, amount_minor, currency, description,
	partner_name, partner_iban, import_batch_id, source_hash,
	raw_data, raw_description, category,
	exclude_from_reports, is_hidden, created_at, updated_at
FROM transactions`

var allowedSortColumns = map[string]bool{
	"effective_date": true,
	"amount_minor":   true,
	"id":             true,
}

var allowedUpdateColumns = map[string]bool{
	"effective_date":       true,
	"amount_minor":         true,
	"currency":             true,
	"description":          true,
	"partner_name":         true,
	"partner_iban":         true,
	"category":             true,
	"exclude_from_reports": true,
	"is_hidden":            true,
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTransaction(s scanner) (*entities.Transaction, error) {
	var (
		id                int64
		effDate           string
		amountMinor       int64
		currencyStr       string
		description       string
		partnerName       sql.NullString
		partnerIBAN       sql.NullString
		importBatchID     sql.NullInt64
		sourceHash        string
		rawData           []byte
		rawDescription    sql.NullString
		category          string
		excludeFromRep    int
		isHidden          int
		createdAtStr      string
		updatedAtStr      string
	)
	err := s.Scan(
		&id, &effDate, &amountMinor, &currencyStr, &description,
		&partnerName, &partnerIBAN, &importBatchID, &sourceHash,
		&rawData, &rawDescription, &category,
		&excludeFromRep, &isHidden, &createdAtStr, &updatedAtStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan transaction: %w", err)
	}
	cur := valueobjects.Currency(currencyStr)
	amount, err := valueobjects.New(amountMinor, cur)
	if err != nil {
		return nil, fmt.Errorf("amount: %w", err)
	}
	return &entities.Transaction{
		ID:                 id,
		EffectiveDate:      effDate,
		Amount:             amount,
		Description:        description,
		PartnerName:        nullStringToPtr(partnerName),
		PartnerIBAN:        nullStringToPtr(partnerIBAN),
		ImportBatchID:      nullInt64ToPtr(importBatchID),
		SourceHash:         sourceHash,
		RawData:            rawData,
		RawDescription:     nullStringToPtr(rawDescription),
		Category:           category,
		ExcludeFromReports: excludeFromRep != 0,
		IsHidden:           isHidden != 0,
		CreatedAt:          parseISO(createdAtStr),
		UpdatedAt:          parseISO(updatedAtStr),
	}, nil
}

func buildWhere(f ports.TxFilters) (string, []any) {
	var clauses []string
	var args []any
	if f.StartDate != nil {
		clauses = append(clauses, "effective_date >= ?")
		args = append(args, *f.StartDate)
	}
	if f.EndDate != nil {
		clauses = append(clauses, "effective_date <= ?")
		args = append(args, *f.EndDate)
	}
	if f.IsHidden != nil {
		clauses = append(clauses, "is_hidden = ?")
		args = append(args, boolToInt(*f.IsHidden))
	}
	if f.ExcludeFromReports != nil {
		clauses = append(clauses, "exclude_from_reports = ?")
		args = append(args, boolToInt(*f.ExcludeFromReports))
	}
	if f.Category != nil {
		clauses = append(clauses, "category = ?")
		args = append(args, *f.Category)
	}
	if len(f.Categories) > 0 {
		placeholders := strings.Repeat("?,", len(f.Categories))
		placeholders = strings.TrimRight(placeholders, ",")
		clauses = append(clauses, "category IN ("+placeholders+")")
		for _, c := range f.Categories {
			args = append(args, c)
		}
	}
	if f.PartnerName != nil {
		clauses = append(clauses, "partner_name = ?")
		args = append(args, *f.PartnerName)
	}
	if f.PartnerIBAN != nil {
		clauses = append(clauses, "partner_iban = ?")
		args = append(args, *f.PartnerIBAN)
	}
	if f.AmountMinMinor != nil {
		clauses = append(clauses, "amount_minor >= ?")
		args = append(args, *f.AmountMinMinor)
	}
	if f.AmountMaxMinor != nil {
		clauses = append(clauses, "amount_minor <= ?")
		args = append(args, *f.AmountMaxMinor)
	}
	if f.AmountSign != nil {
		switch *f.AmountSign {
		case "positive", "pos":
			clauses = append(clauses, "amount_minor > 0")
		case "negative", "neg":
			clauses = append(clauses, "amount_minor < 0")
		}
	}
	if f.DescriptionLike != nil {
		clauses = append(clauses, "description LIKE ?")
		args = append(args, "%"+*f.DescriptionLike+"%")
	}
	if len(f.IDs) > 0 {
		placeholders := strings.Repeat("?,", len(f.IDs))
		placeholders = strings.TrimRight(placeholders, ",")
		clauses = append(clauses, "id IN ("+placeholders+")")
		for _, id := range f.IDs {
			args = append(args, id)
		}
	}
	if len(f.ExcludeIDs) > 0 {
		placeholders := strings.Repeat("?,", len(f.ExcludeIDs))
		placeholders = strings.TrimRight(placeholders, ",")
		clauses = append(clauses, "id NOT IN ("+placeholders+")")
		for _, id := range f.ExcludeIDs {
			args = append(args, id)
		}
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func nullStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func nullInt64(n *int64) any {
	if n == nil {
		return nil
	}
	return *n
}

func nullBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func nullStringToPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

func nullInt64ToPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func timeToISO(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func parseISO(s string) time.Time {
	t, err := time.Parse("2006-01-02T15:04:05.000Z", s)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return time.Time{}
		}
	}
	return t.UTC()
}