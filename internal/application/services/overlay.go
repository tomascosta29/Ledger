package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type OverlayService struct {
	db        *sql.DB
	now       func() time.Time
}

func NewOverlayService(db *sql.DB) *OverlayService {
	return &OverlayService{
		db:  db,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *OverlayService) tableExists(ctx context.Context, tx *sql.Tx, name string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check table %s: %w", name, err)
	}
	return n > 0, nil
}

func (s *OverlayService) Rebuild(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rebuild tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.RebuildWithTx(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *OverlayService) RebuildWithTx(ctx context.Context, tx *sql.Tx) error {
	now := s.now().Format("2006-01-02T15:04:05.000Z")

	hasGroupsTable, err := s.tableExists(ctx, tx, "transaction_groups")
	if err != nil {
		return err
	}
	hasMembersTable, err := s.tableExists(ctx, tx, "transaction_group_members")
	if err != nil {
		return err
	}
	hasGroups := hasGroupsTable && hasMembersTable

	if _, err := tx.ExecContext(ctx, `DELETE FROM overlay_transactions`); err != nil {
		return fmt.Errorf("clear overlay: %w", err)
	}

	if err := s.insertSplitHeaders(ctx, tx, now); err != nil {
		return err
	}
	if err := s.insertRawRows(ctx, tx, now, hasGroups); err != nil {
		return err
	}
	if err := s.insertSplitChildren(ctx, tx, now); err != nil {
		return err
	}
	if hasGroups {
		if err := s.checkGroupCurrencies(ctx, tx); err != nil {
			return err
		}
		if err := s.insertGroupRows(ctx, tx, now); err != nil {
			return err
		}
	}

	return nil
}

func (s *OverlayService) insertSplitHeaders(ctx context.Context, tx *sql.Tx, refreshedAt string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO overlay_transactions (
			effective_date, amount_minor, currency, description,
			partner_name, partner_iban, category, tags,
			source_kind, raw_transaction_id, refreshed_at
		)
		SELECT
			p.effective_date,
			COALESCE(SUM(c.amount_minor), 0),
			p.currency,
			p.description,
			p.partner_name,
			p.partner_iban,
			'Unassigned',
			'',
			'split_header',
			p.id,
			?
		FROM transactions p
		JOIN transactions c ON c.parent_transaction_id = p.id
		GROUP BY p.id
	`, refreshedAt)
	if err != nil {
		return fmt.Errorf("insert split_headers: %w", err)
	}
	return nil
}

func (s *OverlayService) insertRawRows(ctx context.Context, tx *sql.Tx, refreshedAt string, hasGroups bool) error {
	var q string
	if hasGroups {
		q = `
			INSERT INTO overlay_transactions (
				effective_date, amount_minor, currency, description,
				partner_name, partner_iban, category, tags,
				source_kind, raw_transaction_id, exclude_from_reports, refreshed_at
			)
			SELECT
				t.effective_date, t.amount_minor, t.currency, t.description,
				t.partner_name, t.partner_iban, t.category,
				COALESCE((SELECT GROUP_CONCAT(tt.tag, ',') FROM transaction_tags tt WHERE tt.transaction_id = t.id), ''),
				'raw', t.id, t.exclude_from_reports, ?
			FROM transactions t
			WHERE t.is_hidden = 0
			  AND NOT EXISTS (SELECT 1 FROM transactions c WHERE c.parent_transaction_id = t.id)
			  AND NOT EXISTS (SELECT 1 FROM transaction_group_members m WHERE m.transaction_id = t.id)
		`
	} else {
		q = `
			INSERT INTO overlay_transactions (
				effective_date, amount_minor, currency, description,
				partner_name, partner_iban, category, tags,
				source_kind, raw_transaction_id, exclude_from_reports, refreshed_at
			)
			SELECT
				t.effective_date, t.amount_minor, t.currency, t.description,
				t.partner_name, t.partner_iban, t.category,
				COALESCE((SELECT GROUP_CONCAT(tt.tag, ',') FROM transaction_tags tt WHERE tt.transaction_id = t.id), ''),
				'raw', t.id, t.exclude_from_reports, ?
			FROM transactions t
			WHERE t.is_hidden = 0
			  AND NOT EXISTS (SELECT 1 FROM transactions c WHERE c.parent_transaction_id = t.id)
		`
	}
	if _, err := tx.ExecContext(ctx, q, refreshedAt); err != nil {
		return fmt.Errorf("insert raw rows: %w", err)
	}
	return nil
}

func (s *OverlayService) insertSplitChildren(ctx context.Context, tx *sql.Tx, refreshedAt string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO overlay_transactions (
			effective_date, amount_minor, currency, description,
			partner_name, partner_iban, category, tags,
			parent_overlay_id, source_kind, raw_transaction_id, refreshed_at
		)
		SELECT
			c.effective_date, c.amount_minor, c.currency, c.description,
			c.partner_name, c.partner_iban, c.category,
			COALESCE((SELECT GROUP_CONCAT(tt.tag, ',') FROM transaction_tags tt WHERE tt.transaction_id = c.id), ''),
			h.id, 'split_child', c.id, ?
		FROM transactions c
		JOIN overlay_transactions h
		  ON h.raw_transaction_id = c.parent_transaction_id
		 AND h.source_kind = 'split_header'
		WHERE c.parent_transaction_id IS NOT NULL
		  AND c.is_hidden = 0
	`, refreshedAt)
	if err != nil {
		return fmt.Errorf("insert split_children: %w", err)
	}
	return nil
}

func (s *OverlayService) checkGroupCurrencies(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT g.id, g.type, COUNT(DISTINCT t.currency) AS currency_count
		FROM transaction_groups g
		JOIN transaction_group_members m ON m.group_id = g.id
		JOIN transactions t ON t.id = m.transaction_id
		GROUP BY g.id
		HAVING currency_count > 1
	`)
	if err != nil {
		return fmt.Errorf("check group currencies: %w", err)
	}
	defer rows.Close()

	var mixed []string
	for rows.Next() {
		var id int64
		var gtype, currencies string
		if err := rows.Scan(&id, &gtype, &currencies); err != nil {
			return err
		}
		mixed = append(mixed, fmt.Sprintf("group %d (type=%s, %s currencies)", id, gtype, currencies))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(mixed) > 0 {
		return fmt.Errorf("cannot build overlay: groups with mixed currencies: %s", strings.Join(mixed, "; "))
	}
	return nil
}

func (s *OverlayService) insertGroupRows(ctx context.Context, tx *sql.Tx, refreshedAt string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO overlay_transactions (
			effective_date, amount_minor, currency, description,
			partner_name, partner_iban, category, tags,
			group_id, group_role, source_kind, raw_transaction_ids, exclude_from_reports, refreshed_at
		)
		SELECT
			COALESCE(MAX(t.effective_date), ''),
			COALESCE(SUM(t.amount_minor), 0),
			MAX(t.currency),
			'[' || g.type || '] ' || COALESCE(MAX(t.description), ''),
			NULL,
			NULL,
			'Unassigned',
			'',
			g.id,
			g.type,
			CASE WHEN g.type = 'transfer' THEN 'transfer_group' ELSE 'reimbursement_group' END,
			GROUP_CONCAT(t.id),
			CASE WHEN g.type = 'transfer' THEN 1 ELSE 0 END,
			?
		FROM transaction_groups g
		JOIN transaction_group_members m ON m.group_id = g.id
		JOIN transactions t ON t.id = m.transaction_id
		GROUP BY g.id
	`, refreshedAt)
	if err != nil {
		return fmt.Errorf("insert group rows: %w", err)
	}
	return nil
}