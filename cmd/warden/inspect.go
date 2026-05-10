package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	inspectJSON    bool
	inspectVerbose int
	inspectExtract string
)

var inspectCmd = &cobra.Command{
	Use:   "inspect <path>",
	Args:  cobra.ExactArgs(1),
	Short: "Verify and display a build ledger",
	Long: `Parse, verify, and display the contents of a BuildWarden ledger.

The path can be a ledger file directly, or a warden output directory
(the ledger will be found automatically, compressed or not).

Verifies the cryptographic signature chain and displays a summary of all
recorded network requests and artifacts. Exits 0 if the ledger is valid,
exits 1 if verification fails.`,
	Example: `  warden inspect warden-output
  warden inspect warden-output/ledger.zst
  warden inspect --json warden-output
  warden inspect --verbosity 1 ledger.bin`,
	RunE: runInspect,
}

func init() {
	inspectCmd.Flags().BoolVar(&inspectJSON, "json", false, "output as JSON")
	inspectCmd.Flags().IntVar(&inspectVerbose, "verbosity", 0,
		"verbosity: 0=compact, 1=tree, 2=full")
	inspectCmd.Flags().StringVar(&inspectExtract, "extract", "",
		"extract captured payloads to directory")
	rootCmd.AddCommand(inspectCmd)
}

func runInspect(cmd *cobra.Command, args []string) error {
	path := resolveLedgerPath(args[0])

	err := runInspectImpl(path, inspectOptions{
		JSON:      inspectJSON,
		Verbosity: inspectVerbose,
		Writer:    os.Stdout,
		Extract:   inspectExtract,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return nil
}

func resolveLedgerPath(path string) string {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return path
	}
	for _, name := range []string{"ledger.zst", "ledger"} {
		candidate := filepath.Join(path, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return path
}
