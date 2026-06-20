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

var _ ports.DBTX = (*DB)(nil)

type AuditLogRepository struct {
	db *DB
}

func NewAuditLogRepository(db *DB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

func (r *AuditLogRepository) Append(ctx context.Context, entry *entities.AuditEntry) (int64, error) {
	return r.AppendDBTX(ctx, r.db, entry)
}

func (r *AuditLogRepository) AppendDBTX(ctx context.Context, db ports.DBTX, entry *entities.AuditEntry) (int64, error) {
	res, err := db.ExecContext(ctx, `
		INSERT INTO audit_log (table_name, record_id, action, field, old_value, new_value, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		entry.TableName, entry.RecordID, entry.Action,
		nullStr(entry.Field), nullStr(entry.OldValue), nullStr(entry.NewValue),
		timeToISO(entry.CreatedAt),
	)
	if err != nil {
		return 0, fmt.Errorf("insert audit entry: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	entry.ID = id
	return id, nil
}

func (r *AuditLogRepository) AppendBatch(ctx context.Context, entries []*entities.AuditEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO audit_log (table_name, record_id, action, field, old_value, new_value, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, e := range entries {
		if _, err := stmt.ExecContext(ctx,
			e.TableName, e.RecordID, e.Action,
			nullStr(e.Field), nullStr(e.OldValue), nullStr(e.NewValue),
			timeToISO(e.CreatedAt),
		); err != nil {
			return fmt.Errorf("exec audit: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (r *AuditLogRepository) Query(ctx context.Context, filter ports.AuditEntryFilter) ([]*entities.AuditEntry, error) {
	var clauses []string
	var args []any
	if filter.TableName != nil {
		clauses = append(clauses, "table_name = ?")
		args = append(args, *filter.TableName)
	}
	if filter.RecordID != nil {
		clauses = append(clauses, "record_id = ?")
		args = append(args, *filter.RecordID)
	}
	if filter.Action != nil {
		clauses = append(clauses, "action = ?")
		args = append(args, *filter.Action)
	}
	if filter.Since != nil {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, *filter.Since)
	}
	q := `SELECT id, table_name, record_id, action, field, old_value, new_value, created_at FROM audit_log`
	if len(clauses) > 0 {
		q += " WHERE " + strings.Join(clauses, " AND ")
	}
	q += " ORDER BY created_at DESC, id DESC"
	if filter.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit: %w", err)
	}
	defer rows.Close()
	out := make([]*entities.AuditEntry, 0, 32)
	for rows.Next() {
		var (
			e         entities.AuditEntry
			field     sql.NullString
			oldValue  sql.NullString
			newValue  sql.NullString
			createdAt string
		)
		if err := rows.Scan(&e.ID, &e.TableName, &e.RecordID, &e.Action, &field, &oldValue, &newValue, &createdAt); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		e.Field = nullStringToPtr(field)
		e.OldValue = nullStringToPtr(oldValue)
		e.NewValue = nullStringToPtr(newValue)
		e.CreatedAt = parseISO(createdAt)
		out = append(out, &e)
	}
	return out, rows.Err()
}

func (r *AuditLogRepository) LastBatch(ctx context.Context, tableName string, recordID int64) ([]*entities.AuditEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, table_name, record_id, action, field, old_value, new_value, created_at
		FROM audit_log
		WHERE table_name = ? AND record_id = ?
		  AND created_at = (
		    SELECT created_at FROM audit_log
		    WHERE table_name = ? AND record_id = ?
		    ORDER BY created_at DESC, id DESC LIMIT 1
		  )
		ORDER BY id DESC
	`, tableName, recordID, tableName, recordID)
	if err != nil {
		return nil, fmt.Errorf("query batch: %w", err)
	}
	defer rows.Close()
	out := make([]*entities.AuditEntry, 0, 8)
	for rows.Next() {
		var (
			e         entities.AuditEntry
			field     sql.NullString
			oldValue  sql.NullString
			newValue  sql.NullString
			createdAt string
		)
		if err := rows.Scan(&e.ID, &e.TableName, &e.RecordID, &e.Action, &field, &oldValue, &newValue, &createdAt); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		e.Field = nullStringToPtr(field)
		e.OldValue = nullStringToPtr(oldValue)
		e.NewValue = nullStringToPtr(newValue)
		e.CreatedAt = parseISO(createdAt)
		out = append(out, &e)
	}
	if errors.Is(rows.Err(), sql.ErrNoRows) {
		return nil, nil
	}
	return out, rows.Err()
}

func (r *AuditLogRepository) LatestTimestamp(ctx context.Context) (time.Time, error) {
	row := r.db.QueryRowContext(ctx, `SELECT created_at FROM audit_log ORDER BY created_at DESC, id DESC LIMIT 1`)
	var s string
	if err := row.Scan(&s); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return parseISO(s), nil
}

func (r *AuditLogRepository) GetByTimestamp(ctx context.Context, timestamp string) ([]*entities.AuditEntry, error) {
	return r.GetByTimestampDBTX(ctx, r.db, timestamp)
}

func (r *AuditLogRepository) GetByTimestampDBTX(ctx context.Context, db ports.DBTX, timestamp string) ([]*entities.AuditEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, table_name, record_id, action, field, old_value, new_value, created_at
		FROM audit_log
		WHERE created_at = ?
		ORDER BY id DESC
	`, timestamp)
	if err != nil {
		return nil, fmt.Errorf("query audit by timestamp: %w", err)
	}
	defer rows.Close()
	out := make([]*entities.AuditEntry, 0, 8)
	for rows.Next() {
		var (
			e         entities.AuditEntry
			field     sql.NullString
			oldValue  sql.NullString
			newValue  sql.NullString
			createdAt string
		)
		if err := rows.Scan(&e.ID, &e.TableName, &e.RecordID, &e.Action, &field, &oldValue, &newValue, &createdAt); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		e.Field = nullStringToPtr(field)
		e.OldValue = nullStringToPtr(oldValue)
		e.NewValue = nullStringToPtr(newValue)
		e.CreatedAt = parseISO(createdAt)
		out = append(out, &e)
	}
	return out, rows.Err()
}

func (r *AuditLogRepository) FindLatestUndoneTimestamp(ctx context.Context) (string, error) {
	return r.FindLatestUndoneTimestampDBTX(ctx, r.db)
}

func (r *AuditLogRepository) FindLatestUndoneTimestampDBTX(ctx context.Context, db ports.DBTX) (string, error) {
	// Query for the latest timestamp that has not been undone and is not an undo itself.
	row := db.QueryRowContext(ctx, `
		SELECT created_at FROM audit_log
		WHERE action != 'undo'
		  AND created_at NOT IN (
		    SELECT old_value FROM audit_log WHERE action = 'undo' AND old_value IS NOT NULL
		  )
		ORDER BY created_at DESC, id DESC LIMIT 1
	`)
	var s string
	if err := row.Scan(&s); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("find latest undone timestamp: %w", err)
	}
	return s, nil
}
