// Command driver-conformance runs the driver contract's conformance suite
// (specs/driver-contract/v1.md) against a driver binary:
//
//	driver-conformance --driver <path>
//
// It prints one line per check, with the requirements the check holds the
// driver to, and exits 1 when a check failed.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	driverconformance "github.com/buildwarden/buildwarden/conformance/driver"
)

func main() {
	path := flag.String("driver", "", "the driver binary, warden-driver-<name>")
	flag.Parse()
	if *path == "" {
		fmt.Fprintln(os.Stderr, "driver-conformance: --driver is required")
		os.Exit(2)
	}
	s, err := driverconformance.New(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "driver-conformance:", err)
		os.Exit(2)
	}
	failed := false
	for _, r := range s.Static() {
		status := "ok  "
		switch {
		case r.Failure != "":
			status, failed = "FAIL", true
		case r.Skipped != "":
			status = "skip"
		}
		fmt.Printf("%s static: %s (%s)", status, r.Check, strings.Join(r.Requirements,
			", "))
		if msg := r.Failure + r.Skipped; msg != "" {
			fmt.Printf(": %s", msg)
		}
		fmt.Println()
	}
	_ = s.Close()
	if failed {
		os.Exit(1)
	}
}
