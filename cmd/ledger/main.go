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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/tomascosta29/Ledger/internal/application/commands"
	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
	"github.com/tomascosta29/Ledger/internal/infrastructure/persistence"
	tui "github.com/tomascosta29/Ledger/internal/tui"
	tuiScreens "github.com/tomascosta29/Ledger/internal/tui/screens"
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
	rootCmd.AddCommand(categoryCmd)
	rootCmd.AddCommand(budgetCmd)
	rootCmd.AddCommand(recipeCmd)
	rootCmd.AddCommand(summaryCmd)
	rootCmd.AddCommand(ruleCmd)
	rootCmd.AddCommand(transfersCmd)
	rootCmd.AddCommand(reimburseCmd)
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
	categoryRepo := persistence.NewCategoryRepository(db)

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
	if txn.CategoryID == nil {
		fmt.Printf("  Category:    (uncategorized)\n")
	} else if c, err := categoryRepo.GetByID(ctx, *txn.CategoryID); err == nil {
		fmt.Printf("  Category:    %s\n", c.Name)
	}
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

var recipeCmd = &cobra.Command{
	Use:   "recipe",
	Short: "Manage summary recipes",
}

var summaryCmd = &cobra.Command{
	Use:   "summary [--recipe NAME] [--month YYYY-MM]",
	Short: "Print a summary using a recipe (defaults to the active recipe)",
	Long: `summary applies a summary recipe to a month's transactions and prints
income, expense, and net per currency. Use --recipe to pick a recipe
inline; otherwise the active recipe (set via 'ledger recipe use') is
used. Use --month to pick a month; default is the current month.

A recipe is a TOML file in $LEDGER_RECIPES_DIR (default
~/.config/ledger/recipes/*.toml).`,
	Args: cobra.NoArgs,
	RunE: runSummary,
}

func runSummary(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	recipe, _ := cmd.Flags().GetString("recipe")
	month, _ := cmd.Flags().GetString("month")

	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	svc := services.NewSummaryService(services.SummaryDeps{
		OverlayRepo: persistence.NewOverlayRepository(db),
		RecipeRepo:  persistence.NewRecipeRepository(db),
		TagRepo:     persistence.NewTagRepository(db),
	})
	result, err := svc.Run(ctx, recipe, month)
	if err != nil {
		return err
	}
	fmt.Printf("Summary for %s (recipe: %s)\n\n", result.Month, result.RecipeName)
	fmt.Printf("  %-8s  %14s  %14s  %14s  %s\n", "CURRENCY", "INCOME", "EXPENSE", "NET", "TX")
	for _, l := range result.Lines {
		income, _ := valueobjects.New(l.Income, l.Currency)
		expense, _ := valueobjects.New(l.Expense, l.Currency)
		net, _ := valueobjects.New(l.Net, l.Currency)
		fmt.Printf("  %-8s  %14s  %14s  %14s  %d\n",
			l.Currency,
			income.DecimalString()+" "+string(l.Currency),
			expense.DecimalString()+" "+string(l.Currency),
			net.DecimalString()+" "+string(l.Currency),
			l.Count,
		)
	}
	if len(result.Lines) == 0 {
		fmt.Println("  (no transactions matched this recipe for the given month)")
	}
	return nil
}

func init() {
	summaryCmd.Flags().StringP("recipe", "r", "", "recipe name (default: active recipe)")
	summaryCmd.Flags().StringP("month", "m", "", "month in YYYY-MM (default: current month)")
	recipeCmd.AddCommand(recipeListCmd)
	recipeCmd.AddCommand(recipeShowCmd)
	recipeCmd.AddCommand(recipeUseCmd)
	recipeCmd.AddCommand(recipeNewCmd)
}

var recipeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recipes",
	Args:  cobra.NoArgs,
	RunE:  runRecipeList,
}

func runRecipeList(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewRecipeRepository(db)
	all, err := repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	active, _ := repo.GetActiveName(ctx)
	if len(all) == 0 {
		fmt.Printf("no recipes found in %s\n", os.Getenv("LEDGER_RECIPES_DIR"))
		return nil
	}
	fmt.Printf("%-30s  %s\n", "NAME", "DESCRIPTION")
	for _, r := range all {
		marker := "  "
		if r.Name == active {
			marker = "* "
		}
		fmt.Printf("%s%-28s  %s\n", marker, r.Name, r.Description)
	}
	return nil
}

var recipeShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a recipe",
	Args:  cobra.ExactArgs(1),
	RunE:  runRecipeShow,
}

func runRecipeShow(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewRecipeRepository(db)
	r, err := repo.LoadByName(ctx, args[0])
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("recipe %q not found", args[0])
		}
		return err
	}
	fmt.Printf("name        = %q\n", r.Name)
	if r.Description != "" {
		fmt.Printf("description = %q\n", r.Description)
	}
	fmt.Printf("net         = %v\n", r.Net)
	for _, c := range r.Include {
		fmt.Printf("include     = %s %s %q\n", c.Field, c.Op, c.Value)
	}
	for _, c := range r.Exclude {
		fmt.Printf("exclude     = %s %s %q\n", c.Field, c.Op, c.Value)
	}
	return nil
}

var recipeUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Make a recipe the active one",
	Args:  cobra.ExactArgs(1),
	RunE:  runRecipeUse,
}

func runRecipeUse(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewRecipeRepository(db)
	if _, err := repo.LoadByName(ctx, args[0]); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("recipe %q not found", args[0])
		}
		return err
	}
	if err := repo.SetActiveName(ctx, args[0]); err != nil {
		return err
	}
	fmt.Printf("✓ active recipe set to %q\n", args[0])
	return nil
}

var recipeNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Create a new empty recipe file",
	Args:  cobra.ExactArgs(1),
	RunE:  runRecipeNew,
}

func runRecipeNew(cmd *cobra.Command, args []string) error {
	db, err := openDB(cmd.Context())
	if err != nil {
		return err
	}
	defer db.Close()
	_ = db
	dir := os.Getenv("LEDGER_RECIPES_DIR")
	if dir == "" {
		dir = filepath.Join(mustHome(), ".config", "ledger", "recipes")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, args[0]+".toml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("recipe file %s already exists", path)
	}
	contents := fmt.Sprintf(`name = %q
description = ""
include = []
exclude = []
net = false
`, args[0])
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return err
	}
	fmt.Printf("✓ created %s\n", path)
	return nil
}

func mustHome() string {
	h, _ := os.UserHomeDir()
	if h == "" {
		return "."
	}
	return h
}

var ruleCmd = &cobra.Command{
	Use:   "rule",
	Short: "Manage annotation rules",
}

var (
	ruleCreatePriority       int
	ruleCreatePartner        string
	ruleCreateDescription    string
	ruleCreateAmountMin      string
	ruleCreateAmountMax      string
	ruleCreateCategory       string
	ruleCreateBucket         string
	ruleCreateTags           string
	ruleCreateDisabled       bool
)

func init() {
	ruleCmd.AddCommand(ruleListCmd)
	ruleCmd.AddCommand(ruleCreateCmd)
	ruleCmd.AddCommand(ruleDeleteCmd)
	ruleCmd.AddCommand(ruleApplyCmd)
	ruleCreateCmd.Flags().IntVar(&ruleCreatePriority, "priority", 0, "rule priority (higher = applied first)")
	ruleCreateCmd.Flags().StringVar(&ruleCreatePartner, "partner", "", "match: partner name (exact, case-insensitive)")
	ruleCreateCmd.Flags().StringVar(&ruleCreateDescription, "description", "", "match: description substring (case-insensitive)")
	ruleCreateCmd.Flags().StringVar(&ruleCreateAmountMin, "min", "", "match: amount >= <n> (major units, e.g. 100.00)")
	ruleCreateCmd.Flags().StringVar(&ruleCreateAmountMax, "max", "", "match: amount <= <n> (major units)")
	ruleCreateCmd.Flags().StringVar(&ruleCreateCategory, "category", "", "set: category (only if currently Unknown)")
	ruleCreateCmd.Flags().StringVar(&ruleCreateBucket, "bucket", "", "set: bucket name (only if currently unset)")
	ruleCreateCmd.Flags().StringVar(&ruleCreateTags, "add-tags", "", "set: comma-separated tags to add (only if not present)")
	ruleCreateCmd.Flags().BoolVar(&ruleCreateDisabled, "disabled", false, "create the rule disabled")
}

var ruleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List rules",
	Args:  cobra.NoArgs,
	RunE:  runRuleList,
}

func runRuleList(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewRuleRepository(db)
	all, err := repo.List(ctx, false)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Println("no rules")
		return nil
	}
	fmt.Printf("%-4s  %-4s  %-7s  %-20s  %s\n", "ID", "PRI", "ENABLED", "MATCH", "EFFECT")
	for _, r := range all {
		match := formatRuleMatch(*r)
		effect := formatRuleEffect(*r)
		fmt.Printf("%-4d  %-4d  %-7s  %-20s  %s\n",
			r.ID, r.Priority, ifElse(r.Enabled, "yes", "no"),
			match, effect)
	}
	return nil
}

func formatRuleMatch(r entities.Rule) string {
	var parts []string
	if r.MatchPartner != nil {
		parts = append(parts, "partner="+*r.MatchPartner)
	}
	if r.MatchDescription != nil {
		parts = append(parts, "desc~"+*r.MatchDescription)
	}
	if r.MatchAmountMin != nil {
		parts = append(parts, fmt.Sprintf("min=%d", *r.MatchAmountMin))
	}
	if r.MatchAmountMax != nil {
		parts = append(parts, fmt.Sprintf("max=%d", *r.MatchAmountMax))
	}
	if len(parts) == 0 {
		return "(any)"
	}
	return strings.Join(parts, " ")
}

func formatRuleEffect(r entities.Rule) string {
	var parts []string
	if r.SetCategory != nil {
		parts = append(parts, "cat="+*r.SetCategory)
	}
	if r.SetBucketID != nil {
		parts = append(parts, fmt.Sprintf("bucket=%d", *r.SetBucketID))
	}
	if len(r.AddTags) > 0 {
		parts = append(parts, "+tags="+strings.Join(r.AddTags, ","))
	}
	if len(parts) == 0 {
		return "(no-op)"
	}
	return strings.Join(parts, " ")
}

var ruleCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a rule",
	Args:  cobra.ExactArgs(1),
	RunE:  runRuleCreate,
}

func runRuleCreate(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	rule := &entities.Rule{
		Name:     args[0],
		Priority: ruleCreatePriority,
		Enabled:  !ruleCreateDisabled,
	}
	if ruleCreatePartner != "" {
		s := ruleCreatePartner
		rule.MatchPartner = &s
	}
	if ruleCreateDescription != "" {
		s := ruleCreateDescription
		rule.MatchDescription = &s
	}
	if ruleCreateAmountMin != "" {
		v, err := valueobjects.ParseDecimal(ruleCreateAmountMin, valueobjects.EUR)
		if err != nil {
			return fmt.Errorf("min: %w", err)
		}
		v2 := v.Amount
		rule.MatchAmountMin = &v2
	}
	if ruleCreateAmountMax != "" {
		v, err := valueobjects.ParseDecimal(ruleCreateAmountMax, valueobjects.EUR)
		if err != nil {
			return fmt.Errorf("max: %w", err)
		}
		v2 := v.Amount
		rule.MatchAmountMax = &v2
	}
	if ruleCreateCategory != "" {
		s := ruleCreateCategory
		rule.SetCategory = &s
	}
	if ruleCreateBucket != "" {
		bucketRepo := persistence.NewBucketRepository(db)
		b, err := bucketRepo.GetByName(ctx, ruleCreateBucket)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				return fmt.Errorf("bucket %q not found", ruleCreateBucket)
			}
			return err
		}
		id := b.ID
		rule.SetBucketID = &id
	}
	if ruleCreateTags != "" {
		rule.AddTags = strings.Split(ruleCreateTags, ",")
		for i, t := range rule.AddTags {
			rule.AddTags[i] = strings.TrimSpace(t)
		}
	}

	repo := persistence.NewRuleRepository(db)
	id, err := repo.Create(ctx, rule)
	if err != nil {
		return err
	}
	fmt.Printf("✓ rule %d created: %s\n", id, rule.Name)
	return nil
}

var ruleDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a rule",
	Args:  cobra.ExactArgs(1),
	RunE:  runRuleDelete,
}

func runRuleDelete(cmd *cobra.Command, args []string) error {
	id, err := parseInt64List(args[0])
	if err != nil {
		return err
	}
	if len(id) != 1 {
		return fmt.Errorf("rule delete takes a single id")
	}
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewRuleRepository(db)
	if err := repo.Delete(ctx, id[0]); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("rule %d not found", id[0])
		}
		return err
	}
	fmt.Printf("✓ rule %d deleted\n", id[0])
	return nil
}

var ruleApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply all enabled rules to all transactions",
	Args:  cobra.NoArgs,
	RunE:  runRuleApply,
}

var (
	ruleApplyOverwrite bool
)

func init() {
	ruleApplyCmd.Flags().BoolVar(&ruleApplyOverwrite, "overwrite", false, "allow rules to change existing category / bucket / tag assignments (audit-logged as rule_apply)")
}

func runRuleApply(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	annSvc := services.NewAnnotationService(services.AnnotationDeps{
		DB:          db.DB,
		TxRepo:      persistence.NewTransactionRepository(db),
		TagRepo:     persistence.NewTagRepository(db),
		BucketRepo:  persistence.NewBucketRepository(db),
		CategoryRepo: persistence.NewCategoryRepository(db),
		AuditRepo:   persistence.NewAuditLogRepository(db),
		BatchRepo:   persistence.NewImportBatchRepository(db),
		OverlaySvc:  services.NewOverlayService(db.DB),
	})
	ruleSvc := services.NewRuleService(services.RuleDeps{
		TxRepo:      persistence.NewTransactionRepository(db),
		TagRepo:     persistence.NewTagRepository(db),
		BucketRepo:  persistence.NewBucketRepository(db),
		CategoryRepo: persistence.NewCategoryRepository(db),
		RuleRepo:   persistence.NewRuleRepository(db),
		AnnService: annSvc,
	})
	result, err := ruleSvc.Apply(ctx, ruleApplyOverwrite)
	if err != nil {
		return err
	}
	if ruleApplyOverwrite {
		fmt.Printf("✓ rules applied (overwrite): %d matched, %d applied, %d skipped (no-op)\n",
			result.Matched, result.Applied, result.Skipped)
	} else {
		fmt.Printf("✓ rules applied: %d matched, %d applied, %d skipped (no-op)\n",
			result.Matched, result.Applied, result.Skipped)
	}
	for id, n := range result.ByRule {
		if n == 0 {
			continue
		}
		fmt.Printf("  rule %d → %d tx\n", id, n)
	}
	if len(result.Errors) > 0 {
		fmt.Printf("  %d errors:\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Printf("    - %v\n", e)
		}
	}
	return nil
}

var transfersCmd = &cobra.Command{
	Use:   "transfers",
	Short: "Detect and confirm transfer pairs",
}

var reimburseCmd = &cobra.Command{
	Use:   "reimburse",
	Short: "Link an expense with a reimbursement",
}

func init() {
	transfersCmd.AddCommand(transfersDetectCmd)
	transfersCmd.AddCommand(transfersConfirmCmd)
	reimburseCmd.AddCommand(reimburseLinkCmd)
}

var transfersDetectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Heuristically find transfer pairs (opposite signs, same amount, close in time)",
	Args:  cobra.NoArgs,
	RunE:  runTransfersDetect,
}

func runTransfersDetect(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	svc := services.NewTransferService(services.TransferDetectionDeps{
		TxRepo:     persistence.NewTransactionRepository(db),
		GroupRepo:  persistence.NewGroupRepository(db),
		AuditRepo:  persistence.NewAuditLogRepository(db),
		OverlaySvc: services.NewOverlayService(db.DB),
	})
	cands, err := svc.Detect(ctx)
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		fmt.Println("no transfer candidates found")
		return nil
	}
	fmt.Printf("%-5s  %-5s  %-12s  %-12s  %-12s  %-12s  %-5s\n",
		"OUTID", "INID", "OUTDATE", "INDATE", "OUTAMT", "INAMT", "SCORE")
	for _, c := range cands {
		fmt.Printf("%-5d  %-5d  %-12s  %-12s  %12d  %12d  %d\n",
			c.OutID, c.InID, c.OutDate, c.InDate, c.OutAmount, c.InAmount, c.Score)
	}
	fmt.Println()
	fmt.Println("Run 'ledger transfers confirm <outID> <inID>' to mark a pair as a transfer.")
	return nil
}

var transfersConfirmCmd = &cobra.Command{
	Use:   "confirm <outID> <inID>",
	Short: "Mark a transfer pair as linked",
	Args:  cobra.ExactArgs(2),
	RunE:  runTransfersConfirm,
}

func runTransfersConfirm(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	outID, err := parseInt64List(args[0])
	if err != nil || len(outID) != 1 {
		return fmt.Errorf("bad outID")
	}
	inID, err := parseInt64List(args[1])
	if err != nil || len(inID) != 1 {
		return fmt.Errorf("bad inID")
	}
	svc := services.NewTransferService(services.TransferDetectionDeps{
		TxRepo:     persistence.NewTransactionRepository(db),
		GroupRepo:  persistence.NewGroupRepository(db),
		AuditRepo:  persistence.NewAuditLogRepository(db),
		OverlaySvc: services.NewOverlayService(db.DB),
	})
	groupID, err := svc.Confirm(ctx, services.TransferCandidate{
		OutID: outID[0], InID: inID[0],
	})
	if err != nil {
		return err
	}
	fmt.Printf("✓ transfer group %d created (out=%d in=%d)\n", groupID, outID[0], inID[0])
	return nil
}

var reimburseLinkCmd = &cobra.Command{
	Use:   "link <expenseID> <reimbursementID>",
	Short: "Link an expense with its reimbursement",
	Args:  cobra.ExactArgs(2),
	RunE:  runReimburseLink,
}

func runReimburseLink(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	ids, err := parseInt64List(args[0] + "," + args[1])
	if err != nil {
		return err
	}
	svc := services.NewReimbursementService(services.ReimbursementDeps{
		TxRepo:     persistence.NewTransactionRepository(db),
		GroupRepo:  persistence.NewGroupRepository(db),
		AuditRepo:  persistence.NewAuditLogRepository(db),
		OverlaySvc: services.NewOverlayService(db.DB),
	})
	groupID, err := svc.Link(ctx, ids)
	if err != nil {
		return err
	}
	fmt.Printf("✓ reimbursement group %d: %v\n", groupID, ids)
	return nil
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open the LedgerPro TUI (default if no subcommand given)",
	RunE:  runTUI,
}

func runTUI(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	deps := tuiScreens.Deps{
		DB:          db.DB,
		DBPath:      persistence.DefaultDBPath(),
		TxRepo:      persistence.NewTransactionRepository(db),
		TagRepo:     persistence.NewTagRepository(db),
		BucketRepo:  persistence.NewBucketRepository(db),
		AuditRepo:   persistence.NewAuditLogRepository(db),
		BatchRepo:   persistence.NewImportBatchRepository(db),
		GroupRepo:   persistence.NewGroupRepository(db),
		OverlayRepo: persistence.NewOverlayRepository(db),
		OverlaySvc:  services.NewOverlayService(db.DB),
		BudgetSvc:   persistence.NewBucketRepository(db),
		RecipeSvc:   persistence.NewRecipeRepository(db),
	}

	app := tui.NewApp(ctx, deps)
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err = p.Run()
	return err
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
		DB:          db.DB,
		TxRepo:      persistence.NewTransactionRepository(db),
		TagRepo:     persistence.NewTagRepository(db),
		BucketRepo:  persistence.NewBucketRepository(db),
		CategoryRepo: persistence.NewCategoryRepository(db),
		AuditRepo:   persistence.NewAuditLogRepository(db),
		BatchRepo:   persistence.NewImportBatchRepository(db),
		OverlaySvc:  services.NewOverlayService(db.DB),
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

var categoryCmd = &cobra.Command{
	Use:   "category",
	Short: "Manage the curated category set",
}

var (
	categoryListArchived bool
)

func init() {
	categoryCmd.AddCommand(categoryListCmd)
	categoryListCmd.Flags().BoolVar(&categoryListArchived, "all", false, "include archived categories")
}

var categoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List categories",
	Args:  cobra.NoArgs,
	RunE:  runCategoryList,
}

func runCategoryList(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persistence.NewCategoryRepository(db)
	cats, err := repo.List(ctx, categoryListArchived)
	if err != nil {
		return err
	}
	if len(cats) == 0 {
		fmt.Println("no categories")
		return nil
	}
	fmt.Printf("%-6s  %-22s  %s\n", "ID", "NAME", "STATUS")
	for _, c := range cats {
		status := "active"
		if c.ArchivedAt != nil {
			status = fmt.Sprintf("archived %s", c.ArchivedAt.Format("2006-01-02"))
		}
		fmt.Printf("%-6d  %-22s  %s\n", c.ID, c.Name, status)
	}
	return nil
}

var (
	categoryCreateDescription string
)

func init() {
	categoryCmd.AddCommand(categoryCreateCmd)
	categoryCmd.AddCommand(categoryRenameCmd)
	categoryCmd.AddCommand(categoryArchiveCmd)
	categoryCreateCmd.Flags().StringVarP(&categoryCreateDescription, "description", "d", "", "description for the new category")
}

var categoryCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a category",
	Args:  cobra.ExactArgs(1),
	RunE:  runCategoryCreate,
}

func runCategoryCreate(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	svc := services.NewCategoryService(services.CategoryDeps{
		DB:          db.DB,
		CategoryRepo: persistence.NewCategoryRepository(db),
		AuditRepo:   persistence.NewAuditLogRepository(db),
	})
	c, err := svc.Create(ctx, args[0], categoryCreateDescription)
	if err != nil {
		return err
	}
	fmt.Printf("created category %q (id=%d)\n", c.Name, c.ID)
	return nil
}

var categoryRenameCmd = &cobra.Command{
	Use:   "rename <old> <new>",
	Short: "Rename a category",
	Args:  cobra.ExactArgs(2),
	RunE:  runCategoryRename,
}

func runCategoryRename(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	svc := services.NewCategoryService(services.CategoryDeps{
		DB:          db.DB,
		CategoryRepo: persistence.NewCategoryRepository(db),
		AuditRepo:   persistence.NewAuditLogRepository(db),
	})
	if err := svc.Rename(ctx, args[0], args[1]); err != nil {
		return err
	}
	fmt.Printf("renamed %q -> %q\n", args[0], args[1])
	return nil
}

var categoryArchiveCmd = &cobra.Command{
	Use:   "archive <name>",
	Short: "Archive a category (hide from listings; undo restores it)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCategoryArchive,
}

func runCategoryArchive(cmd *cobra.Command, args []string) error {
	ctx := ctxFromCmd(cmd)
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	svc := services.NewCategoryService(services.CategoryDeps{
		DB:          db.DB,
		CategoryRepo: persistence.NewCategoryRepository(db),
		AuditRepo:   persistence.NewAuditLogRepository(db),
	})
	if err := svc.Archive(ctx, args[0]); err != nil {
		return err
	}
	fmt.Printf("archived category %q\n", args[0])
	return nil
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
		DB:          db.DB,
		TxRepo:      persistence.NewTransactionRepository(db),
		TagRepo:     persistence.NewTagRepository(db),
		BucketRepo:  persistence.NewBucketRepository(db),
		CategoryRepo: persistence.NewCategoryRepository(db),
		AuditRepo:   persistence.NewAuditLogRepository(db),
		BatchRepo:   persistence.NewImportBatchRepository(db),
		OverlaySvc:  services.NewOverlayService(db.DB),
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
