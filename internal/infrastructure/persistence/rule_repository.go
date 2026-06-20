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
)

type RuleRepository struct {
	db *sql.DB
}

func NewRuleRepository(db *DB) *RuleRepository {
	return &RuleRepository{db: db.DB}
}

func (r *RuleRepository) Create(ctx context.Context, rule *entities.Rule) (int64, error) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO rules (name, priority, match_partner, match_description,
			match_amount_min, match_amount_max, set_category, set_bucket_id,
			add_tags, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.Name,
		rule.Priority,
		nullStr(rule.MatchPartner),
		nullStr(rule.MatchDescription),
		nullInt64(rule.MatchAmountMin),
		nullInt64(rule.MatchAmountMax),
		nullStr(rule.SetCategory),
		nullInt64(rule.SetBucketID),
		strings.Join(rule.AddTags, ","),
		boolToInt(rule.Enabled),
		now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create rule: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

func (r *RuleRepository) GetByID(ctx context.Context, id int64) (*entities.Rule, error) {
	row := r.db.QueryRowContext(ctx, selectAllRulesSQL+" WHERE id = ?", id)
	return scanRule(row)
}

func (r *RuleRepository) List(ctx context.Context, enabledOnly bool) ([]*entities.Rule, error) {
	q := selectAllRulesSQL
	if enabledOnly {
		q += " WHERE enabled = 1"
	}
	q += " ORDER BY priority DESC, id"
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()
	out := make([]*entities.Rule, 0, 4)
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *RuleRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

const selectAllRulesSQL = `SELECT
	id, name, priority, match_partner, match_description,
	match_amount_min, match_amount_max, set_category, set_bucket_id,
	add_tags, enabled, created_at, updated_at
FROM rules`

func scanRule(s scanner) (*entities.Rule, error) {
	var (
		r           entities.Rule
		matchPart   sql.NullString
		matchDesc   sql.NullString
		amountMin   sql.NullInt64
		amountMax   sql.NullInt64
		setCategory sql.NullString
		setBucket   sql.NullInt64
		addTags     sql.NullString
		enabled     int
		createdAt   string
		updatedAt   string
	)
	err := s.Scan(
		&r.ID, &r.Name, &r.Priority, &matchPart, &matchDesc,
		&amountMin, &amountMax, &setCategory, &setBucket,
		&addTags, &enabled, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, fmt.Errorf("scan rule: %w", err)
	}
	r.MatchPartner = nullStringToPtr(matchPart)
	r.MatchDescription = nullStringToPtr(matchDesc)
	r.MatchAmountMin = nullInt64ToPtr(amountMin)
	r.MatchAmountMax = nullInt64ToPtr(amountMax)
	r.SetCategory = nullStringToPtr(setCategory)
	r.SetBucketID = nullInt64ToPtr(setBucket)
	if addTags.Valid && addTags.String != "" {
		r.AddTags = strings.Split(addTags.String, ",")
	}
	r.Enabled = enabled != 0
	r.CreatedAt = parseISO(createdAt)
	r.UpdatedAt = parseISO(updatedAt)
	return &r, nil
}
