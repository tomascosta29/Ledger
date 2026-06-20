package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tomascosta29/Ledger/internal/application/commands"
	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
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
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(rebuildOverlayCmd)
	importCmd.Flags().StringVarP(&importProfile, "profile", "p", "", "bank profile (erste, revolut, or custom TOML)")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "parse and preview without writing to the DB")
	listCmd.Flags().IntVar(&listLimit, "limit", 50, "max number of rows to show")
	listCmd.Flags().StringVar(&listCategory, "category", "", "filter by category")
	listCmd.Flags().BoolVar(&listIncludeHidden, "include-hidden", false, "include hidden transactions")
	listCmd.Flags().StringVar(&listSince, "since", "", "filter rows effective on or after YYYY-MM-DD")
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
	fmt.Println("  ledger import <file.csv> --profile erste|revolut")
	fmt.Println("                                       import a CSV statement")
	fmt.Println("  ledger --help                       full command list")
	return nil
}

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import a CSV bank statement",
	Long: `import reads a CSV file using the named profile, parses it into Raw
Transactions, computes source hashes for deduplication, and inserts new
rows into the ledger. Re-imports of the same file are skipped (no
duplicates). An audit log entry is written for each inserted transaction.

Use --dry-run to preview without writing to the DB.

Profiles:
  erste    Erste Bank (Austrian) CSV export
  revolut  Revolut CSV export
  custom   any *.toml in $LEDGER_PROFILE_DIR (default ~/.config/ledger/profiles)`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

var (
	importProfile string
	importDryRun  bool
)

func runImport(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if importProfile == "" {
		return fmt.Errorf("--profile is required (e.g. --profile erste)")
	}
	file := args[0]
	if _, err := os.Stat(file); err != nil {
		return fmt.Errorf("input file: %w", err)
	}

	dbPath := persistence.DefaultDBPath()
	db, err := persistence.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	uc := commands.NewImportUseCase(commands.ImportDeps{
		TxRepo:     persistence.NewTransactionRepository(db),
		BatchRepo:  persistence.NewImportBatchRepository(db),
		AuditRepo:  persistence.NewAuditLogRepository(db),
		OverlaySvc: services.NewOverlayService(db.DB),
	})

	result, err := uc.Execute(ctx, commands.ImportOptions{
		File:        file,
		ProfileName: importProfile,
		SourceFile:  filepath.Base(file),
		DryRun:      importDryRun,
	})
	if err != nil {
		return err
	}

	mode := "imported"
	if importDryRun {
		mode = "would import"
	}
	fmt.Printf("✓ %s %s: read=%d inserted=%s skipped=%d\n",
		mode, file, result.Stats.RowsRead,
		fmtInt(result.Stats.RowsInserted), result.Stats.RowsSkipped)
	if importDryRun {
		dupes := 0
		for _, p := range result.Preview {
			if p.Duplicate {
				dupes++
			}
		}
		fmt.Printf("  duplicates detected: %d\n", dupes)
		fmt.Println("  re-run without --dry-run to insert")
	}
	return nil
}

func fmtInt(n int) string {
	return fmt.Sprintf("%d", n)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List imported transactions",
	Long: `list prints the most recent transactions from the ledger. By default
hidden transactions are excluded. Filters: --category, --since, --include-hidden.`,
	RunE: runList,
}

var (
	listLimit         int
	listCategory      string
	listIncludeHidden bool
	listSince         string
)

func runList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	dbPath := persistence.DefaultDBPath()
	db, err := persistence.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	repo := persistence.NewOverlayRepository(db)
	filters := ports.OverlayFilters{
		SourceKinds: []ports.SourceKind{ports.SourceRaw, ports.SourceSplitChild, ports.SourceReimbursementGroup},
	}
	if listCategory != "" {
		filters.Category = &listCategory
	}
	if listSince != "" {
		filters.StartDate = &listSince
	}
	if listIncludeHidden {
		filters.SourceKinds = append(filters.SourceKinds, ports.SourceSplitHeader)
	}

	rows, err := repo.FindAll(ctx, ports.OverlayFindOptions{
		Filters: filters,
		Sort:    ports.OverlaySortByDate,
		Order:   ports.SortDesc,
		Limit:   listLimit,
	})
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	if len(rows) == 0 {
		fmt.Println("no transactions (overlay is empty — run `ledger rebuild-overlay` if you've imported data but never rebuilt)")
		return nil
	}

	fmt.Printf("%-6s  %-10s  %12s  %-12s  %-20s  %s\n", "ID", "DATE", "AMOUNT", "KIND", "PARTNER", "DESCRIPTION")
	for _, o := range rows {
		partner := ""
		if o.PartnerName != nil {
			partner = *o.PartnerName
		}
		desc := o.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		fmt.Printf("%-6d  %-10s  %12s  %-12s  %-20s  %s\n",
			o.ID, o.EffectiveDate, o.Amount.DecimalString()+" "+string(o.Amount.Currency),
			string(o.SourceKind), truncate(partner, 20), desc)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

var rebuildOverlayCmd = &cobra.Command{
	Use:   "rebuild-overlay",
	Short: "Force a full rebuild of the overlay table from raw transactions",
	Long: `rebuild-overlay deletes and repopulates overlay_transactions from the raw
transactions + groups + annotations. Normally this happens automatically
on every annotation write. Use this command after manual DB edits or
when you suspect the overlay is stale.`,
	RunE: runRebuildOverlay,
}

func runRebuildOverlay(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	dbPath := persistence.DefaultDBPath()
	db, err := persistence.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	svc := services.NewOverlayService(db.DB)
	if err := svc.Rebuild(ctx); err != nil {
		return err
	}

	repo := persistence.NewOverlayRepository(db)
	n, err := repo.Count(ctx, ports.OverlayFilters{})
	if err != nil {
		return fmt.Errorf("count: %w", err)
	}
	fmt.Printf("✓ overlay rebuilt: %d rows\n", n)
	return nil
}