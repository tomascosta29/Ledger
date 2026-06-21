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

type GroupRepository struct {
	db *sql.DB
}

func NewGroupRepository(db *DB) *GroupRepository {
	return &GroupRepository{db: db.DB}
}

func (r *GroupRepository) CreateGroup(ctx context.Context, g *entities.TransactionGroup) (int64, error) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO transaction_groups (name, created_at) VALUES (?, ?)`,
		g.Name, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create group: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

func (r *GroupRepository) GetGroup(ctx context.Context, id int64) (*entities.TransactionGroup, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM transaction_groups WHERE id = ?`, id)
	var g entities.TransactionGroup
	var createdAt string
	if err := row.Scan(&g.ID, &g.Name, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	g.CreatedAt = parseISO(createdAt)
	return &g, nil
}

func (r *GroupRepository) AddMember(ctx context.Context, groupID, txID int64, role string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO transaction_group_members (group_id, transaction_id, role) VALUES (?, ?, ?)`,
		groupID, txID, role,
	)
	if err != nil {
		return fmt.Errorf("add group member: %w", err)
	}
	return nil
}

func (r *GroupRepository) RemoveMember(ctx context.Context, groupID, txID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM transaction_group_members WHERE group_id = ? AND transaction_id = ?`,
		groupID, txID,
	)
	return err
}

func (r *GroupRepository) ListMembers(ctx context.Context, groupID int64) ([]*entities.GroupMember, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT group_id, transaction_id, role FROM transaction_group_members WHERE group_id = ?`,
		groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*entities.GroupMember, 0, 4)
	for rows.Next() {
		var m entities.GroupMember
		if err := rows.Scan(&m.GroupID, &m.TransactionID, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (r *GroupRepository) ListGroups(ctx context.Context) ([]*entities.TransactionGroup, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, created_at FROM transaction_groups ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*entities.TransactionGroup, 0, 4)
	for rows.Next() {
		var g entities.TransactionGroup
		var createdAt string
		if err := rows.Scan(&g.ID, &g.Name, &createdAt); err != nil {
			return nil, err
		}
		g.CreatedAt = parseISO(createdAt)
		out = append(out, &g)
	}
	return out, rows.Err()
}
