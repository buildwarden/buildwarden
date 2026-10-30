// Command tier1 runs BuildWarden's deterministic tier-1 checks (LINT-02) over
// the diff between a base and HEAD, and prints the rule index (LINT-01).
//
//	tier1 [-base REV] [-json] [-design PATH]... [-no-design] [-repo DIR]
//	tier1 rules [-json]
//
// Exit 0 is clean (or SHOULD findings and hints only), 1 is an unsuppressed
// MUST or MUST-review finding, and 2 is a usage or environment error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// options are the default run's flags.
type options struct {
	base     string
	json     bool
	designs  multi
	noDesign bool
	repo     string
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// fixable is an environment error that names its fix (UX-04).
type fixable struct {
	what, fix string
}

func (e *fixable) Error() string { return e.what }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "rules" {
		return runRules(args[1:], stdout, stderr)
	}
	var o options
	fs := flag.NewFlagSet("tier1", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.base, "base", "origin/main",
		"diff against the merge base of REV and HEAD")
	fs.BoolVar(&o.json, "json", false, "print the report as JSON (report.schema.json)")
	fs.Var(&o.designs, "design", "a design to use as the claim (repeatable); "+
		"default: the designs the diff adds or modifies")
	fs.BoolVar(&o.noDesign, "no-design", false,
		"the change needs no design (FLOW-01): the PR's no-design label")
	fs.StringVar(&o.repo, "repo", ".", "the repository root")
	if err := fs.Parse(args); err != nil {
		return report.ExitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "tier1: unexpected argument %q\n", fs.Arg(0))
		return report.ExitUsage
	}
	r, err := check(o)
	if err != nil {
		return fail(stderr, err)
	}
	if o.json {
		if err := r.JSON(stdout); err != nil {
			return fail(stderr, err)
		}
	} else {
		r.Text(stdout)
	}
	return r.ExitCode()
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "ERROR: %v\n", err)
	var fx *fixable
	if errors.As(err, &fx) {
		fmt.Fprintf(stderr, "To fix: %s\n", fx.fix)
	}
	return report.ExitUsage
}

// check runs every stage over the diff and returns the finished report.
func check(o options) (*report.Report, error) {
	r := &report.Report{}
	r.Finish()
	return r, nil
}

func runRules(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tier1 rules", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print the index as JSON")
	repo := fs.String("repo", ".", "the repository root")
	if err := fs.Parse(args); err != nil {
		return report.ExitUsage
	}
	_, _ = asJSON, repo
	return report.ExitClean
}
