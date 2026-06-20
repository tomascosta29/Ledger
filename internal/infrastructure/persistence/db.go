package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultBusyTimeout = 5 * time.Second
	sqliteDriverName   = "sqlite"
)

type DB struct {
	*sql.DB
	path string
}

func (d *DB) Path() string { return d.path }

func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" {
		return nil, fmt.Errorf("db path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(%d)&_pragma=synchronous(NORMAL)",
		path, defaultBusyTimeout.Milliseconds())

	raw, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	raw.SetConnMaxLifetime(0)

	pingCtx, cancel := context.WithTimeout(ctx, defaultBusyTimeout)
	defer cancel()
	if err := raw.PingContext(pingCtx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	return &DB{DB: raw, path: path}, nil
}

func DefaultDBPath() string {
	if p := os.Getenv("LEDGER_DB_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "ledger.db"
	}
	return filepath.Join(home, ".local", "share", "ledger", "ledger.db")
}