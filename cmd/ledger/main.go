package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
	Version:      version,
	SilenceUsage: true,
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
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("ledger init: not yet implemented")
		return nil
	},
}