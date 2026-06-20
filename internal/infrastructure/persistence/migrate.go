package persistence

import (
	"database/sql"
	"embed"
	"fmt"
	"io"
	"log"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func init() {
	goose.SetLogger(log.New(io.Discard, "", 0))
}

func Migrate(db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}

func MigrationStatus(db *sql.DB) (int64, error) {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return 0, err
	}
	return goose.GetDBVersion(db)
}

func Down(db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.Down(db, "migrations"); err != nil {
		return fmt.Errorf("goose down: %w", err)
	}
	return nil
}

func Reset(db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.Reset(db, "migrations"); err != nil {
		return fmt.Errorf("goose reset: %w", err)
	}
	return nil
}
