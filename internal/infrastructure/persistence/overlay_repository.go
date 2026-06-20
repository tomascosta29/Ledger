package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
)

type OverlayRepository struct {
	db *DB
}

func NewOverlayRepository(db *DB) *OverlayRepository {
	return &OverlayRepository{db: db}
}

const overlaySelectColumnsSQL = `SELECT
	id, effective_date, amount_minor, currency, description,
	partner_name, partner_iban, category, bucket_id, tags,
	parent_overlay_id, group_id, group_role, source_kind,
	raw_transaction_id, raw_transaction_ids, exclude_from_reports, refreshed_at
FROM overlay_transactions`

func (r *OverlayRepository) GetByID(ctx context.Context, id int64) (*ports.OverlayTransaction, error) {
	row := r.db.QueryRowContext(ctx, overlaySelectColumnsSQL+" WHERE id = ?", id)
	return scanOverlay(row)
}

func (r *OverlayRepository) FindAll(ctx context.Context, opts ports.OverlayFindOptions) ([]*ports.OverlayTransaction, error) {
	where, args := buildOverlayWhere(opts.Filters)
	q := overlaySelectColumnsSQL + where
	if opts.Sort != "" {
		sortCol := string(opts.Sort)
		if !overlaySortAllowed[sortCol] {
			return nil, fmt.Errorf("invalid overlay sort column: %s", sortCol)
		}
		order := "ASC"
		if opts.Order == ports.SortDesc {
			order = "DESC"
		}
		q += fmt.Sprintf(" ORDER BY %s %s", sortCol, order)
	} else {
		q += " ORDER BY effective_date DESC, id DESC"
	}
	if opts.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query overlay: %w", err)
	}
	defer rows.Close()
	out := make([]*ports.OverlayTransaction, 0, 32)
	for rows.Next() {
		o, err := scanOverlay(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *OverlayRepository) Count(ctx context.Context, filters ports.OverlayFilters) (int64, error) {
	where, args := buildOverlayWhere(filters)
	var n int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM overlay_transactions"+where, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count overlay: %w", err)
	}
	return n, nil
}

var overlaySortAllowed = map[string]bool{
	"effective_date": true,
	"amount_minor":   true,
	"id":             true,
}

func buildOverlayWhere(f ports.OverlayFilters) (string, []any) {
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
		clauses = append(clauses, "exclude_from_reports = ?")
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
	if len(f.SourceKinds) > 0 {
		placeholders := strings.Repeat("?,", len(f.SourceKinds))
		placeholders = strings.TrimRight(placeholders, ",")
		clauses = append(clauses, "source_kind IN ("+placeholders+")")
		for _, k := range f.SourceKinds {
			args = append(args, string(k))
		}
	}
	if f.GroupID != nil {
		clauses = append(clauses, "group_id = ?")
		args = append(args, *f.GroupID)
	}
	if f.ParentOverlayID != nil {
		clauses = append(clauses, "parent_overlay_id = ?")
		args = append(args, *f.ParentOverlayID)
	}
	if f.RawTransactionID != nil {
		clauses = append(clauses, "raw_transaction_id = ?")
		args = append(args, *f.RawTransactionID)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanOverlay(s scanner) (*ports.OverlayTransaction, error) {
	var (
		id               int64
		effDate          string
		amountMinor      int64
		currencyStr      string
		description      string
		partnerName      sql.NullString
		partnerIBAN      sql.NullString
		category         string
		bucketID         sql.NullInt64
		tags             string
		parentOverlayID  sql.NullInt64
		groupID          sql.NullInt64
		groupRole        sql.NullString
		sourceKind       string
		rawTxnID         sql.NullInt64
		rawTxnIDs        sql.NullString
		excludeFromRep   int
		refreshedAtStr   string
	)
	err := s.Scan(
		&id, &effDate, &amountMinor, &currencyStr, &description,
		&partnerName, &partnerIBAN, &category, &bucketID, &tags,
		&parentOverlayID, &groupID, &groupRole, &sourceKind,
		&rawTxnID, &rawTxnIDs, &excludeFromRep, &refreshedAtStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan overlay: %w", err)
	}
	cur := valueobjects.Currency(currencyStr)
	amount, amtErr := valueobjects.New(amountMinor, cur)
	if amtErr != nil {
		return nil, fmt.Errorf("amount: %w", amtErr)
	}
	return &ports.OverlayTransaction{
		ID:                 id,
		EffectiveDate:      effDate,
		Amount:             amount,
		Description:        description,
		PartnerName:        nullStringToPtr(partnerName),
		PartnerIBAN:        nullStringToPtr(partnerIBAN),
		Category:           category,
		BucketID:           nullInt64ToPtr(bucketID),
		Tags:               tags,
		ParentOverlayID:    nullInt64ToPtr(parentOverlayID),
		GroupID:            nullInt64ToPtr(groupID),
		GroupRole:          nullStringToPtr(groupRole),
		SourceKind:         ports.SourceKind(sourceKind),
		RawTransactionID:   nullInt64ToPtr(rawTxnID),
		RawTransactionIDs:  rawTxnIDs.String,
		ExcludeFromReports: excludeFromRep != 0,
		RefreshedAt:        parseISO(refreshedAtStr),
	}, nil
}

var _ ports.OverlayRepository = (*OverlayRepository)(nil)

var _ = time.Time{}