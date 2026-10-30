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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/buildwarden/buildwarden/tools/tier1"
	"github.com/buildwarden/buildwarden/tools/tier1/analyzers"
	"github.com/buildwarden/buildwarden/tools/tier1/coupling"
	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
	"github.com/buildwarden/buildwarden/tools/tier1/rules"
	"github.com/buildwarden/buildwarden/tools/tier1/surface"
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
type fixable interface {
	error
	Fix() string
}

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
	var fx fixable
	if errors.As(err, &fx) {
		fmt.Fprintf(stderr, "To fix: %s\n", fx.Fix())
	}
	return report.ExitUsage
}

// check runs every stage over the diff and returns the finished report.
func check(o options) (*report.Report, error) {
	d, err := diff.Load(o.repo, o.base)
	if err != nil {
		return nil, err
	}
	ix, cfg, err := loadRules(o.repo)
	if err != nil {
		return nil, err
	}
	r := &report.Report{Base: d.Base, Head: d.Head}
	r.Add(d.Bad...)
	if touchedUnder(d, rules.Dir+"/", "tools/tier1/rules.yaml") {
		r.Add(rules.SelfCheck(ix, cfg)...)
	}
	c := &coupling.Checker{Diff: d, Cfg: cfg, Index: ix,
		Designs: coupling.LoadDesigns(d, o.designs), NoDesign: o.noDesign}
	r.Add(c.All()...)
	sf, err := surface.Check(d, cfg.Surface.Dir, cfg.Surface.Docs, ix.Severity)
	if err != nil {
		return nil, err
	}
	r.Add(sf...)
	af, err := analyzers.Run(d, ix.Severity)
	if err != nil {
		return nil, err
	}
	r.Add(d.OnChangedLines(af)...)
	d.Suppress(r)
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
	ix, cfg, err := loadRules(*repo)
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(ix.Rules); err != nil {
			return fail(stderr, err)
		}
	} else {
		for _, r := range ix.Rules {
			fmt.Fprintf(stdout, "%-10s %-11s %-40s %s\n",
				r.ID, r.Severity, r.Check, r.Title)
		}
	}
	problems := rules.SelfCheck(ix, cfg)
	for _, f := range problems {
		fmt.Fprintf(stderr, "%s:%d [%s] %s\n", f.File, f.Line, f.Rule, f.Message)
	}
	if len(problems) > 0 {
		return report.ExitFail
	}
	return report.ExitClean
}

func loadRules(repo string) (*rules.Index, *rules.Config, error) {
	ix, err := rules.Load(repo)
	if err != nil {
		return nil, nil, err
	}
	if len(ix.Rules) == 0 {
		return nil, nil, fmt.Errorf("no rules found under %s/%s; is -repo the repo root?",
			repo, rules.Dir)
	}
	cfg, err := rules.ParseConfig(tier1.RulesYAML)
	return ix, cfg, err
}

// touchedUnder reports whether the diff touched any path equal to, or under
// (for a prefix ending in "/"), one of prefixes.
func touchedUnder(d *diff.Diff, prefixes ...string) bool {
	for _, f := range d.Files {
		for _, p := range prefixes {
			if diff.Match(p, f.Path) || (f.Old != "" && diff.Match(p, f.Old)) {
				return true
			}
		}
	}
	return false
}
