package persistence

import (
	"database/sql"
	"fmt"

	"github.com/tomascosta29/Ledger/internal/application/ports"
)

type SQLUnitOfWork struct {
	db *sql.DB
}

func NewSQLUnitOfWork(db *sql.DB) *SQLUnitOfWork {
	return &SQLUnitOfWork{db: db}
}

func (u *SQLUnitOfWork) WithTx(fn func(tx *sql.Tx) error) error {
	tx, err := u.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

var _ ports.UnitOfWork = (*SQLUnitOfWork)(nil)
