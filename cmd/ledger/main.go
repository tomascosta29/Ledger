package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
)

var version = "0.0.0"

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "ledger",
	Short: "Hyperspecialized personal accounting for one operator",
	Long: `LedgerPro Go — a single binary for one operator's personal accounting.

Primary surface is the Bubble Tea TUI (run with ` + "`ledger tui`" + `).
Use the subcommands below for ops and scripting. See SPEC.md for the v1 plan.`,
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(initCmd)
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open the LedgerPro TUI (default if no subcommand given)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("ledger tui: not yet implemented")
		return nil
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a fresh ledger DB at $LEDGER_DB_PATH (default ~/.local/share/ledger/ledger.db)",
	Long: `init creates the SQLite database at the configured path and runs all migrations.

Refuses to run if the DB file already exists. To re-create, delete the file first.`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	dbPath := persistence.DefaultDBPath()

	if _, err := os.Stat(dbPath); err == nil {
		return fmt.Errorf("db already exists at %s\nremove it first to re-initialize", dbPath)
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	db, err := persistence.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if err := persistence.Migrate(db.DB); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	fmt.Printf("✓ Initialized LedgerPro DB at %s\n", dbPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  ledger tui                          open the interactive TUI")
	fmt.Println("  ledger import <file.csv>            import a CSV statement")
	fmt.Println("  ledger --help                       full command list")
	return nil
}