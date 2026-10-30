// Package report holds tier 1's findings and prints them as text grouped by
// severity or as the stable JSON in report.schema.json (LINT-05, UX-02).
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Severity is a rule's severity from its **Severity:** line, or Hint for a
// mechanical signal that never fails (GLOBAL-04's).
type Severity string

// The severities, in the order the text output prints them.
const (
	Must       Severity = "MUST"
	MustReview Severity = "MUST-review"
	Should     Severity = "SHOULD"
	Hint       Severity = "hint"
)

// Blocking reports whether an unsuppressed finding of this severity fails
// tier 1 (LINT-03).
func (s Severity) Blocking() bool { return s == Must || s == MustReview }

// Exit codes (LINT-03, UX-04).
const (
	ExitClean = 0 // clean, or SHOULD findings and hints only
	ExitFail  = 1 // an unsuppressed MUST or MUST-review finding
	ExitUsage = 2 // a usage or environment error
)

// Finding is one check's result against one rule. Line is 0 for a
// file-level finding.
type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Check    string   `json:"check"`
	Message  string   `json:"message"`
}

// Suppression is a Tier1-Allow trailer that cleared at least one finding.
type Suppression struct {
	Rule   string `json:"rule"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Commit string `json:"commit"`
}

// Report is one run's output.
type Report struct {
	Base       string         `json:"base"`
	Head       string         `json:"head"`
	Findings   []Finding      `json:"findings"`
	Hints      []Finding      `json:"hints"`
	Suppressed []Suppression  `json:"suppressed"`
	Counts     map[string]int `json:"counts"`
}

// Add files f under Hints when its severity is Hint, and under Findings
// otherwise.
func (r *Report) Add(f ...Finding) {
	for _, x := range f {
		if x.Severity == Hint {
			r.Hints = append(r.Hints, x)
		} else {
			r.Findings = append(r.Findings, x)
		}
	}
}

// Finish sorts the report so reruns diff cleanly and fills Counts.
func (r *Report) Finish() {
	for _, fs := range [][]Finding{r.Findings, r.Hints} {
		sort.SliceStable(fs, func(i, j int) bool { return less(fs[i], fs[j]) })
	}
	sort.SliceStable(r.Suppressed, func(i, j int) bool {
		a, b := r.Suppressed[i], r.Suppressed[j]
		return a.Rule+a.Path+a.Commit < b.Rule+b.Path+b.Commit
	})
	r.Counts = map[string]int{string(Must): 0, string(MustReview): 0, string(Should): 0,
		string(Hint): len(r.Hints), "suppressed": len(r.Suppressed)}
	for _, f := range r.Findings {
		r.Counts[string(f.Severity)]++
	}
}

func less(a, b Finding) bool {
	if a.File != b.File {
		return a.File < b.File
	}
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	if a.Rule != b.Rule {
		return a.Rule < b.Rule
	}
	return a.Message < b.Message
}

// ExitCode is ExitFail when any finding blocks, and ExitClean otherwise.
func (r *Report) ExitCode() int {
	for _, f := range r.Findings {
		if f.Severity.Blocking() {
			return ExitFail
		}
	}
	return ExitClean
}

// JSON writes the report in the report.schema.json form. Empty lists are
// written as [] rather than null so consumers need no nil case.
func (r *Report) JSON(w io.Writer) error {
	out := *r
	if out.Findings == nil {
		out.Findings = []Finding{}
	}
	if out.Hints == nil {
		out.Hints = []Finding{}
	}
	if out.Suppressed == nil {
		out.Suppressed = []Suppression{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// Text writes the human form, grouped by severity (UX-02).
func (r *Report) Text(w io.Writer) {
	fmt.Fprintf(w, "tier1: %s...%s\n", short(r.Base), short(r.Head))
	for _, sev := range []Severity{Must, MustReview, Should} {
		var group []Finding
		for _, f := range r.Findings {
			if f.Severity == sev {
				group = append(group, f)
			}
		}
		section(w, string(sev), group)
	}
	section(w, "hints (not failing; for tier 2)", r.Hints)
	if len(r.Suppressed) > 0 {
		fmt.Fprintf(w, "\nsuppressed (Tier1-Allow, %d):\n", len(r.Suppressed))
		for _, s := range r.Suppressed {
			fmt.Fprintf(w, "  %s %s: %s (commit %s)\n",
				s.Rule, s.Path, s.Reason, short(s.Commit))
		}
	}
	fmt.Fprintf(w, "\n%d MUST, %d MUST-review, %d SHOULD, %d hints, %d suppressed\n",
		r.Counts[string(Must)], r.Counts[string(MustReview)], r.Counts[string(Should)],
		r.Counts[string(Hint)], r.Counts["suppressed"])
}

func section(w io.Writer, title string, fs []Finding) {
	if len(fs) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s (%d):\n", title, len(fs))
	for _, f := range fs {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Fprintf(w, "  %s [%s %s] %s\n", loc, f.Rule, f.Check, f.Message)
	}
}

func short(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}
