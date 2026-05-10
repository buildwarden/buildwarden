package main

import (
	"fmt"
	"os"

	"warden/internal/inspect"

	"github.com/spf13/cobra"
)

var (
	inspectJSON    bool
	inspectVerbose int
)

var inspectCmd = &cobra.Command{
	Use:   "inspect <ledger-file>",
	Args:  cobra.ExactArgs(1),
	Short: "Verify and display a build ledger",
	Long: `Parse, verify, and display the contents of a BuildWarden ledger file.

Verifies the cryptographic signature chain and displays a summary of all
recorded network requests and artifacts. Exits 0 if the ledger is valid,
exits 1 if verification fails.`,
	Example: `  warden inspect /tmp/warden-ledger-abc123/ledger
  warden inspect --json ledger.bin
  warden inspect --verbosity 1 ledger.bin`,
	RunE: runInspect,
}

func init() {
	inspectCmd.Flags().BoolVar(&inspectJSON, "json", false, "output as JSON")
	inspectCmd.Flags().IntVar(&inspectVerbose, "verbosity", 0,
		"verbosity: 0=compact, 1=tree, 2=full")
	rootCmd.AddCommand(inspectCmd)
}

func runInspect(cmd *cobra.Command, args []string) error {
	err := inspect.Run(args[0], inspect.Options{
		JSON:      inspectJSON,
		Verbosity: inspectVerbose,
		Writer:    os.Stdout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return nil
}
