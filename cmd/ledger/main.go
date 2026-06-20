package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tomascosta29/Ledger/internal/application/commands"
	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
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
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(splitCmd)
	rootCmd.AddCommand(categorizeCmd)
	rootCmd.AddCommand(hideCmd)
	rootCmd.AddCommand(tagCmd)
	rootCmd.AddCommand(undoCmd)
	rootCmd.AddCommand(bucketCmd)
	rootCmd.AddCommand(budgetCmd)
	importCmd.Flags().StringVarP(&importProfile, "profile", "p", "", "bank profile (erste, revolut, or custom TOML)")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "parse and preview without writing to the DB")
	listCmd.Flags().IntVar(&listLimit, "limit", 50, "max number of rows to show")
	listCmd.Flags().StringVar(&listCategory, "category", "", "filter by category")
	listCmd.Flags().StringVar(&listSince, "since", "", "filter rows effective on or after YYYY-MM-DD")
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a transaction manually (outside the bank statement flow)",
	Long: `add creates a single Raw Transaction with the given fields. The
source hash is computed from the inputs (profile "manual" v1), so
re-running add with the same arguments is a no-op. Use --category to
set the category at insert time; otherwise the default "Unknown"
applies and can be changed with 'ledger categorize'.`,
	Args: cobra.NoArgs,
	RunE: runAdd,
}

func runAdd(cmd *cobra.Command, args []string) error {
	date, _ := cmd.Flags().GetString("date")
	amount, _ := cmd.Flags().GetString("amount")
	currency, _ := cmd.Flags().GetString("currency")
	description, _ := cmd.Flags().GetString("description")
	partner, _ := cmd.Flags().GetString("partner")
	iban, _ := cmd.Flags().GetString("iban")
	category, _ := cmd.Flags().GetString("category")

	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	uc := commands.NewManualAddUseCase(commands.ManualAddDeps{
		TxRepo:     persistence.NewTransactionRepository(db),
		AuditRepo:  persistence.NewAuditLogRepository(db),
		OverlaySvc: services.NewOverlayService(db.DB),
	})
	result, err := uc.Execute(ctx, commands.ManualAddOptions{
		EffectiveDate: date,
		Amount:        amount,
		Currency:      currency,
		Description:   description,
		PartnerName:   partner,
		PartnerIBAN:   iban,
		Category:      category,
	})
	if err != nil {
		return err
	}
	if result.Created {
		fmt.Printf("✓ added transaction %d: %s %s %s (%s)\n",
			result.TransactionID, amount, currency, description, date)
	} else {
		fmt.Printf("✓ transaction %d already exists with these fields (no-op)\n", result.TransactionID)
	}
	return nil
}

func init() {
	addCmd.Flags().StringP("date", "d", "", "effective date (YYYY-MM-DD) (required)")
	addCmd.Flags().StringP("amount", "a", "", "amount in major units, e.g. -42.10 or 100.00 (required)")
	addCmd.Flags().StringP("currency", "c", "", "currency (e.g. EUR) (required)")
	addCmd.Flags().StringP("description", "D", "", "description (required)")
	addCmd.Flags().StringP("partner", "p", "", "partner name (optional)")
	addCmd.Flags().String("iban", "", "partner IBAN (optional)")
	addCmd.Flags().String("category", "", "category (default: Unknown)")
}

var showCmd = &cobra.Command{
	Use:   "show <txID>",
	Short: "Show details of a single transaction",
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

func runShow(cmd *cobra.Command, args []string) error {
	ids, err := parseInt64List(args[0])
	if err != nil {
		return err
	}
	if len(ids) != 1 {
		return fmt.Errorf("show takes a single transaction id")
	}
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	txRepo := persistence.NewTransactionRepository(db)
	tagRepo := persistence.NewTagRepository(db)
	bucketRepo := persistence.NewBucketRepository(db)

	txn, err := txRepo.GetByID(ctx, ids[0])
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("transaction %d not found", ids[0])
		}
		return err
	}

	fmt.Printf("Transaction %d\n", txn.ID)
	fmt.Printf("  Date:        %s\n", txn.EffectiveDate)
	fmt.Printf("  Amount:      %s\n", txn.Amount.String())
	fmt.Printf("  Description: %s\n", txn.Description)
	if txn.PartnerName != nil {
		fmt.Printf("  Partner:     %s\n", *txn.PartnerName)
	}
	if txn.PartnerIBAN != nil {
		fmt.Printf("  IBAN:        %s\n", *txn.PartnerIBAN)
	}
	fmt.Printf("  Category:    %s\n", txn.Category)
	if txn.BucketID != nil {
		bucket, err := bucketRepo.GetByID(ctx, *txn.BucketID)
		if err == nil {
			fmt.Printf("  Bucket:      %s\n", bucket.Name)
		}
	}
	if txn.IsHidden {
		fmt.Printf("  Hidden:      yes\n")
	}
	if txn.ExcludeFromReports {
		fmt.Printf("  Excluded:    yes\n")
	}
	if txn.ImportBatchID != nil {
		fmt.Printf("  Import batch: %d\n", *txn.ImportBatchID)
	}
	if txn.ParentTxnID != nil {
		fmt.Printf("  Split parent: %d\n", *txn.ParentTxnID)
	}
	fmt.Printf("  Source hash: %s\n", txn.SourceHash)
	fmt.Printf("  Created:     %s\n", txn.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Updated:     %s\n", txn.UpdatedAt.Format("2006-01-02 15:04:05"))

	tags, _ := tagRepo.ListByTransaction(ctx, txn.ID)
	if len(tags) > 0 {
		fmt.Printf("  Tags:        %s\n", strings.Join(tags, ", "))
	}
	return nil
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show the audit log",
	Long: `history prints the audit log, one entry per line. Use --tx-id to
filter to a single transaction, --action to filter by action type
(import, categorize, visibility, tag, bucket_assign, split, undo, ...),
and --limit to cap the number of rows (default 50, newest first).`,
	Args: cobra.NoArgs,
	RunE: runHistory,
}

func runHistory(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	txID, _ := cmd.Flags().GetInt64("tx-id")
	action, _ := cmd.Flags().GetString("action")
	limit, _ := cmd.Flags().GetInt("limit")

	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewAuditLogRepository(db)

	filter := ports.AuditEntryFilter{Limit: limit}
	if txID > 0 {
		filter.RecordID = &txID
	}
	if action != "" {
		filter.Action = &action
	}
	entries, err := repo.Query(ctx, filter)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("no audit entries")
		return nil
	}
	fmt.Printf("%-23s  %-5s  %-15s  %-7s  %-10s  %-10s  %s\n", "WHEN", "TX", "ACTION", "FIELD", "OLD", "NEW", "ID")
	for _, e := range entries {
		old, new := "", ""
		if e.OldValue != nil {
			old = truncate(*e.OldValue, 10)
		}
		if e.NewValue != nil {
			new = truncate(*e.NewValue, 10)
		}
		fmt.Printf("%-23s  %-5d  %-15s  %-7s  %-10s  %-10s  %d\n",
			e.CreatedAt.Format("2006-01-02 15:04:05.000"),
			e.RecordID,
			e.Action,
			derefStr(e.Field),
			old, new,
			e.ID,
		)
	}
	return nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func init() {
	historyCmd.Flags().Int64("tx-id", 0, "filter to a single transaction id")
	historyCmd.Flags().String("action", "", "filter by action (import, categorize, visibility, tag, bucket_assign, undo, ...)")
	historyCmd.Flags().Int("limit", 50, "max number of rows to show")
}

var splitCmd = &cobra.Command{
	Use:   "split <txID>",
	Short: "Split a transaction into N children",
	Long: `split breaks a transaction into N children whose amounts sum to the
parent. The parent stays in the transactions table (it's the split
header in the overlay) and each child becomes a split_child overlay row.
Use 'ledger undo' to revert.

Two modes:
  - non-interactive: pass --child "amount|description" once per child
  - interactive:    no --child flags; prompted line by line

Example:
  ledger split 42 --child "-20.00|Groceries" --child "-15.00|Household"`,
	Args: cobra.ExactArgs(1),
	RunE: runSplit,
}

func runSplit(cmd *cobra.Command, args []string) error {
	ids, err := parseInt64List(args[0])
	if err != nil {
		return err
	}
	if len(ids) != 1 {
		return fmt.Errorf("split takes a single transaction id")
	}
	childFlags, _ := cmd.Flags().GetStringSlice("child")

	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	txRepo := persistence.NewTransactionRepository(db)

	children := make([]commands.SplitChild, 0)
	if len(childFlags) > 0 {
		for i, raw := range childFlags {
			amt, desc, err := parseChildSpec(raw)
			if err != nil {
				return fmt.Errorf("child %d: %w", i+1, err)
			}
			cur, err := txRepo.GetByID(ctx, ids[0])
			if err != nil {
				return err
			}
			children = append(children, commands.SplitChild{
				AmountMinor: amt,
				Currency:    cur.Amount.Currency,
				Description: desc,
			})
		}
	} else {
		parent, err := txRepo.GetByID(ctx, ids[0])
		if err != nil {
			return err
		}
		fmt.Printf("Splitting transaction %d (%s %s, %s)\n",
			parent.ID, parent.Amount.String(), parent.EffectiveDate, parent.Description)
		fmt.Println("Enter each child as 'amount|description' (e.g. -20.00|Groceries).")
		fmt.Println("Empty line when done. Last child's amount is computed to match the parent.")
		reader := bufio.NewReader(os.Stdin)
		remain := parent.Amount.Amount
		for {
			fmt.Printf("  child [%s remaining] amount|description: ", formatMinorInt(remain, parent.Amount.Currency))
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			amt, desc, err := parseChildSpec(line)
			if err != nil {
				fmt.Printf("  ! %v\n", err)
				continue
			}
			if amt == 0 {
				fmt.Println("  ! amount cannot be zero")
				continue
			}
			if amt > remain && remain > 0 {
				fmt.Printf("  ! amount exceeds remaining %s\n", formatMinorInt(remain, parent.Amount.Currency))
				continue
			}
			if amt < remain && remain < 0 {
				fmt.Printf("  ! amount below remaining %s (more negative)\n", formatMinorInt(remain, parent.Amount.Currency))
				continue
			}
			children = append(children, commands.SplitChild{
				AmountMinor: amt,
				Currency:    parent.Amount.Currency,
				Description: desc,
			})
			remain -= amt
			if remain == 0 {
				break
			}
		}
	}

	uc := commands.NewSplitUseCase(commands.SplitDeps{
		TxRepo:     txRepo,
		AuditRepo:  persistence.NewAuditLogRepository(db),
		OverlaySvc: services.NewOverlayService(db.DB),
	})
	result, err := uc.Execute(ctx, commands.SplitOptions{
		TransactionID: ids[0],
		Children:      children,
	})
	if err != nil {
		return err
	}
	idsStrs := make([]string, len(result.ChildrenIDs))
	for i, id := range result.ChildrenIDs {
		idsStrs[i] = fmt.Sprintf("%d", id)
	}
	fmt.Printf("✓ split %d into %d children: %s\n",
		result.ParentID, len(result.ChildrenIDs), strings.Join(idsStrs, ", "))
	return nil
}

func parseChildSpec(s string) (int64, string, error) {
	parts := strings.SplitN(s, "|", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return 0, "", fmt.Errorf("expected 'amount|description', got %q", s)
	}
	amountStr := strings.TrimSpace(parts[0])
	desc := strings.TrimSpace(parts[1])
	// Currency-agnostic parse: use EUR as the parser's currency, but we
	// only need the minor amount. The actual currency comes from the parent.
	money, err := valueobjects.ParseDecimal(amountStr, valueobjects.EUR)
	if err != nil {
		return 0, "", fmt.Errorf("amount %q: %w", amountStr, err)
	}
	return money.Amount, desc, nil
}

func formatMinorInt(minor int64, currency valueobjects.Currency) string {
	m := valueobjects.MustNew(minor, currency)
	return m.DecimalString() + " " + string(currency)
}

func init() {
	splitCmd.Flags().StringSlice("child", nil, "child spec 'amount|description' (repeat for each child)")
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
	listLimit    int
	listCategory string
	listSince    string
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
		SourceKinds: []ports.SourceKind{ports.SourceRaw, ports.SourceSplitHeader, ports.SourceSplitChild, ports.SourceReimbursementGroup},
	}
	if listCategory != "" {
		filters.Category = &listCategory
	}
	if listSince != "" {
		filters.StartDate = &listSince
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

var categorizeCmd = &cobra.Command{
	Use:   "categorize <txID[,txID,...]> --category NAME",
	Short: "Set the category on one or more transactions",
	Args:  cobra.ExactArgs(1),
	RunE:  runCategorize,
}

func runCategorize(cmd *cobra.Command, args []string) error {
	ids, err := parseInt64List(args[0])
	if err != nil {
		return err
	}
	category, _ := cmd.Flags().GetString("category")
	if category == "" {
		return fmt.Errorf("--category is required")
	}
	var bucket *string
	if b, _ := cmd.Flags().GetString("bucket"); b != "" {
		bucket = &b
	}
	return runAnnotation(ctxFromCmd(cmd), func(svc *services.AnnotationService) error {
		return svc.BulkCategorize(ctxFromCmd(cmd), ids, category, bucket)
	}, fmt.Sprintf("categorized %d transaction(s) → %q (%s)", len(ids), category, joinIDs(ids)))
}

var hideCmd = &cobra.Command{
	Use:   "hide <txID[,txID,...]>",
	Short: "Hide one or more transactions from queries (they stay in raw for audit)",
	Args:  cobra.ExactArgs(1),
	RunE:  runHide,
}

var hideShow bool

func runHide(cmd *cobra.Command, args []string) error {
	ids, err := parseInt64List(args[0])
	if err != nil {
		return err
	}
	hidden := true
	if unhide, _ := cmd.Flags().GetBool("unhide"); unhide {
		hidden = false
	}
	return runAnnotation(ctxFromCmd(cmd), func(svc *services.AnnotationService) error {
		return svc.BulkSetHidden(ctxFromCmd(cmd), ids, hidden)
	}, fmt.Sprintf("%s %d transaction(s) (%s)", ifElse(hidden, "hidden", "unhidden"), len(ids), joinIDs(ids)))
}

var tagCmd = &cobra.Command{
	Use:   "tag <txID[,txID,...]>",
	Short: "Add or remove tags on one or more transactions",
	Args:  cobra.ExactArgs(1),
	RunE:  runTag,
}

func runTag(cmd *cobra.Command, args []string) error {
	ids, err := parseInt64List(args[0])
	if err != nil {
		return err
	}
	add, _ := cmd.Flags().GetStringSlice("add")
	remove, _ := cmd.Flags().GetStringSlice("remove")
	if len(add) == 0 && len(remove) == 0 {
		return fmt.Errorf("at least one of --add or --remove is required")
	}
	return runAnnotation(ctxFromCmd(cmd), func(svc *services.AnnotationService) error {
		if len(add) > 0 {
			if err := svc.BulkAddTags(ctxFromCmd(cmd), ids, add); err != nil {
				return fmt.Errorf("add tags: %w", err)
			}
		}
		if len(remove) > 0 {
			if err := svc.BulkRemoveTags(ctxFromCmd(cmd), ids, remove); err != nil {
				return fmt.Errorf("remove tags: %w", err)
			}
		}
		return nil
	}, fmt.Sprintf("tagged %d transaction(s) (+%v -%v) (%s)", len(ids), add, remove, joinIDs(ids)))
}

func init() {
	categorizeCmd.Flags().StringP("category", "c", "", "category name (e.g. need, want, savings)")
	categorizeCmd.Flags().StringP("bucket", "b", "", "bucket name to assign (optional)")
	hideCmd.Flags().Bool("unhide", false, "unhide instead of hiding")
	tagCmd.Flags().StringSlice("add", nil, "tag(s) to add (comma-separated)")
	tagCmd.Flags().StringSlice("remove", nil, "tag(s) to remove (comma-separated)")
}

func runAnnotation(ctx context.Context, fn func(*services.AnnotationService) error, successMsg string) error {
	dbPath := persistence.DefaultDBPath()
	db, err := persistence.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	svc := services.NewAnnotationService(services.AnnotationDeps{
		DB:         db.DB,
		TxRepo:     persistence.NewTransactionRepository(db),
		TagRepo:    persistence.NewTagRepository(db),
		BucketRepo: persistence.NewBucketRepository(db),
		AuditRepo:  persistence.NewAuditLogRepository(db),
		BatchRepo:  persistence.NewImportBatchRepository(db),
		OverlaySvc: services.NewOverlayService(db.DB),
	})
	if err := fn(svc); err != nil {
		return err
	}
	fmt.Printf("✓ %s\n", successMsg)
	return nil
}

var undoCmd = &cobra.Command{
	Use:   "undo",
	Short: "Undo the last operation (reverts categories, hidden state, tags, or imports)",
	Args:  cobra.NoArgs,
	RunE:  runUndo,
}

var bucketCmd = &cobra.Command{
	Use:   "bucket",
	Short: "Manage buckets (budget envelopes)",
}

var (
	bucketListArchived bool
)

func init() {
	bucketCmd.AddCommand(bucketListCmd)
	bucketCmd.AddCommand(bucketCreateCmd)
	bucketCmd.AddCommand(bucketUpdateCmd)
	bucketCmd.AddCommand(bucketArchiveCmd)
	bucketCmd.AddCommand(bucketDeleteCmd)
	bucketListCmd.Flags().BoolVar(&bucketListArchived, "all", false, "include archived buckets")
	bucketCreateCmd.Flags().StringP("currency", "c", "", "currency (e.g. EUR)")
	bucketCreateCmd.Flags().Float64P("allocation", "a", 0, "monthly allocation in major units (e.g. 500.00)")
	bucketCreateCmd.MarkFlagRequired("currency")
	bucketCreateCmd.MarkFlagRequired("allocation")
	bucketUpdateCmd.Flags().StringP("name", "n", "", "new name")
	bucketUpdateCmd.Flags().StringP("currency", "c", "", "new currency")
	bucketUpdateCmd.Flags().Float64P("allocation", "a", 0, "new monthly allocation in major units")
	budgetCmd.Flags().StringP("month", "m", "", "month (YYYY-MM); default is current month")
}

var bucketListCmd = &cobra.Command{
	Use:   "list",
	Short: "List buckets",
	Args:  cobra.NoArgs,
	RunE:  runBucketList,
}

func runBucketList(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewBucketRepository(db)
	buckets, err := repo.List(ctx, bucketListArchived)
	if err != nil {
		return err
	}
	if len(buckets) == 0 {
		fmt.Println("no buckets")
		return nil
	}
	fmt.Printf("%-6s  %-22s  %-8s  %14s  %s\n", "ID", "NAME", "CURRENCY", "ALLOCATION", "STATUS")
	for _, b := range buckets {
		status := "active"
		if b.ArchivedAt != nil {
			status = fmt.Sprintf("archived %s", b.ArchivedAt.Format("2006-01-02"))
		}
		cur := valueobjects.Currency(b.Currency)
		alloc, _ := valueobjects.New(b.MonthlyAllocationMinor, cur)
		fmt.Printf("%-6d  %-22s  %-8s  %14s  %s\n",
			b.ID, b.Name, b.Currency, alloc.DecimalString(), status)
	}
	return nil
}

var bucketCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a bucket",
	Args:  cobra.ExactArgs(1),
	RunE:  runBucketCreate,
}

func runBucketCreate(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	name := args[0]
	currency, _ := cmd.Flags().GetString("currency")
	allocation, _ := cmd.Flags().GetFloat64("allocation")
	if currency == "" || allocation == 0 {
		return fmt.Errorf("--currency and --allocation are required")
	}
	cur := valueobjects.Currency(currency)
	money, err := valueobjects.ParseDecimal(fmt.Sprintf("%.2f", allocation), cur)
	if err != nil {
		return fmt.Errorf("allocation: %w", err)
	}
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewBucketRepository(db)
	id, err := repo.Create(ctx, &entities.Bucket{
		Name:                   name,
		Currency:               currency,
		MonthlyAllocationMinor: money.Amount,
	})
	if err != nil {
		return err
	}
	fmt.Printf("✓ bucket %d created: %s (%s, %s / month)\n", id, name, currency, money.DecimalString())
	return nil
}

var bucketUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update a bucket's name, currency, or allocation",
	Args:  cobra.ExactArgs(1),
	RunE:  runBucketUpdate,
}

func runBucketUpdate(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	name := args[0]
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewBucketRepository(db)
	b, err := repo.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("bucket %q not found", name)
		}
		return err
	}
	if newName, _ := cmd.Flags().GetString("name"); newName != "" {
		b.Name = newName
	}
	if newCur, _ := cmd.Flags().GetString("currency"); newCur != "" {
		b.Currency = newCur
	}
	if newAlloc, _ := cmd.Flags().GetFloat64("allocation"); newAlloc != 0 {
		cur := valueobjects.Currency(b.Currency)
		money, err := valueobjects.ParseDecimal(fmt.Sprintf("%.2f", newAlloc), cur)
		if err != nil {
			return fmt.Errorf("allocation: %w", err)
		}
		b.MonthlyAllocationMinor = money.Amount
	}
	if err := repo.Update(ctx, b); err != nil {
		return err
	}
	fmt.Printf("✓ bucket %q updated\n", b.Name)
	return nil
}

var bucketArchiveCmd = &cobra.Command{
	Use:   "archive <name>",
	Short: "Archive a bucket (no longer appears in budget; data is preserved)",
	Args:  cobra.ExactArgs(1),
	RunE:  runBucketArchive,
}

func runBucketArchive(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	name := args[0]
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewBucketRepository(db)
	b, err := repo.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("bucket %q not found", name)
		}
		return err
	}
	if err := repo.Archive(ctx, b.ID); err != nil {
		return err
	}
	fmt.Printf("✓ bucket %q archived\n", name)
	return nil
}

var bucketDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a bucket (only if no transactions are assigned)",
	Args:  cobra.ExactArgs(1),
	RunE:  runBucketDelete,
}

func runBucketDelete(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	name := args[0]
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewBucketRepository(db)
	b, err := repo.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("bucket %q not found", name)
		}
		return err
	}
	if err := repo.Delete(ctx, b.ID); err != nil {
		return err
	}
	fmt.Printf("✓ bucket %q deleted\n", name)
	return nil
}

var budgetCmd = &cobra.Command{
	Use:   "budget [--month YYYY-MM]",
	Short: "Show per-bucket allocation vs spend for a month",
	Long: `budget prints each active bucket with its monthly allocation, the total
spent against it in the given month, and the remaining headroom. Defaults
to the current month. Unassigned spend (no bucket) is summarised at the
end.`,
	Args: cobra.NoArgs,
	RunE: runBudget,
}

func runBudget(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	month, _ := cmd.Flags().GetString("month")
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewBucketRepository(db)
	spends, err := repo.SpendByMonth(ctx, month)
	if err != nil {
		return err
	}
	unassigned, err := repo.UnassignedSpendByMonth(ctx, month)
	if err != nil {
		return err
	}

	fmt.Printf("Budget for %s\n\n", month)
	fmt.Printf("  %-22s  %12s  %12s  %12s  %s\n", "BUCKET", "ALLOCATED", "SPENT", "REMAINING", "TX")
	for _, s := range spends {
		remaining := s.AllocatedMinor - s.SpentMinor
		fmt.Printf("  %-22s  %12s  %12s  %12s  %d\n",
			s.BucketName,
			formatMinor(s.AllocatedMinor, s.Currency),
			formatMinor(s.SpentMinor, s.Currency),
			formatMinor(remaining, s.Currency),
			s.Count,
		)
	}
	if len(unassigned) > 0 {
		fmt.Println("\n  Unassigned:")
		for _, s := range unassigned {
			fmt.Printf("    %-20s  %12s  %d tx\n",
				s.Currency,
				formatMinor(s.SpentMinor, s.Currency),
				s.Count,
			)
		}
	}
	return nil
}

func openDB(ctx context.Context) (*persistence.DB, error) {
	dbPath := persistence.DefaultDBPath()
	db, err := persistence.Open(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return db, nil
}

func formatMinor(minor int64, currency string) string {
	cur := valueobjects.Currency(currency)
	m, err := valueobjects.New(minor, cur)
	if err != nil {
		return fmt.Sprintf("%d %s", minor, currency)
	}
	return m.DecimalString() + " " + currency
}

func runUndo(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	dbPath := persistence.DefaultDBPath()
	db, err := persistence.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	svc := services.NewAnnotationService(services.AnnotationDeps{
		DB:         db.DB,
		TxRepo:     persistence.NewTransactionRepository(db),
		TagRepo:    persistence.NewTagRepository(db),
		BucketRepo: persistence.NewBucketRepository(db),
		AuditRepo:  persistence.NewAuditLogRepository(db),
		BatchRepo:  persistence.NewImportBatchRepository(db),
		OverlaySvc: services.NewOverlayService(db.DB),
	})

	if err := svc.Undo(ctx); err != nil {
		return err
	}
	fmt.Println("✓ undo: last operation reverted successfully")
	return nil
}

func ctxFromCmd(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func ifElse[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
