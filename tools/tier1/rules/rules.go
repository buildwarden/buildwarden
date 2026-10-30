// Package rules extracts the rule index from docs/development/ (LINT-01),
// loads rules.yaml, and self-checks that every automated-tagged rule is
// mapped to one tier (LINT-02).
package rules

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

// Dir is where the standards live, relative to the repo root.
const Dir = "docs/development"

// Rule is one rule header with its metadata lines.
type Rule struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Severity  report.Severity `json:"severity"`
	Check     string          `json:"check"`
	Automated bool            `json:"automated"`
	File      string          `json:"file"`
	Line      int             `json:"line"`
}

// Index is the extracted rule index.
type Index struct {
	Rules []Rule
	// Problems are headers whose Severity or Check line is missing or invalid.
	Problems []report.Finding
}

// Severity is the rule's severity, or MUST for an ID the index lacks, so an
// unknown rule never silently stops blocking.
func (ix *Index) Severity(id string) report.Severity {
	for _, r := range ix.Rules {
		if r.ID == id && r.Severity != "" {
			return r.Severity
		}
	}
	return report.Must
}

var (
	headerRe = regexp.MustCompile(`^### ([A-Z]+-\d{2}) — (.+)$`)
	sevRe    = regexp.MustCompile(`^\*\*Severity:\*\*\s*(.*)$`)
	checkRe  = regexp.MustCompile(`^\*\*Check:\*\*\s*(.*)$`)
	qualRe   = regexp.MustCompile(`\s*\([^)]*\)$`)
)

// Load parses every docs/development/*.md under root except README.md, whose
// RELAY-01 header is the format example.
func Load(root string) (*Index, error) {
	files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(Dir), "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	ix := &Index{}
	for _, f := range files {
		if filepath.Base(f) == "README.md" {
			continue
		}
		text, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		ix.parse(path.Join(Dir, filepath.Base(f)), string(text))
	}
	return ix, nil
}

func (ix *Index) parse(file, text string) {
	var cur *Rule
	finish := func() {
		if cur != nil {
			ix.validate(*cur)
			ix.Rules = append(ix.Rules, *cur)
		}
		cur = nil
	}
	for i, line := range strings.Split(text, "\n") {
		if m := headerRe.FindStringSubmatch(line); m != nil {
			finish()
			cur = &Rule{ID: m[1], Title: m[2], File: file, Line: i + 1}
			continue
		}
		if cur == nil {
			continue
		}
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			finish()
		} else if m := sevRe.FindStringSubmatch(line); m != nil && cur.Severity == "" {
			cur.Severity = report.Severity(strings.TrimSpace(m[1]))
		} else if m := checkRe.FindStringSubmatch(line); m != nil && cur.Check == "" {
			cur.Check = strings.TrimSpace(m[1])
			cur.Automated = strings.Contains(cur.Check, "automated")
		}
	}
	finish()
}

func (ix *Index) validate(r Rule) {
	bad := func(format string, a ...any) {
		ix.Problems = append(ix.Problems, report.Finding{Rule: "LINT-01",
			Severity: report.Must, File: r.File, Line: r.Line, Check: "rule-index",
			Message: r.ID + ": " + fmt.Sprintf(format, a...)})
	}
	switch r.Severity {
	case report.Must, report.MustReview, report.Should:
	case "":
		bad("no **Severity:** line")
	default:
		bad("severity %q is not MUST, MUST-review or SHOULD", r.Severity)
	}
	if r.Check == "" {
		bad("no **Check:** line")
		return
	}
	for _, part := range strings.Split(r.Check, "+") {
		p := qualRe.ReplaceAllString(strings.TrimSpace(part), "")
		if p != "automated" && p != "agent-judgment" {
			bad("check %q: %q is not automated or agent-judgment", r.Check, p)
		}
	}
}

// Mapping is one rules.yaml entry. Exactly one field is set.
type Mapping struct {
	Check    []string `yaml:"check"`
	Owner    string   `yaml:"owner"`
	FollowUp string   `yaml:"follow-up"`
}

// Seam is a versioned seam with a contract document (GLOBAL-08).
type Seam struct {
	Name     string   `yaml:"name"`
	Code     []string `yaml:"code"`
	Contract string   `yaml:"contract"`
}

// Pair is a code path set and the document that changes with it.
type Pair struct {
	Code string `yaml:"code"`
	Doc  string `yaml:"doc"`
}

// Config is rules.yaml.
type Config struct {
	Rules   map[string]Mapping `yaml:"rules"`
	NotCode []string           `yaml:"not_code"`
	Drivers struct {
		All    []string `yaml:"all"`
		Dirs   []string `yaml:"dirs"`
		Shared []string `yaml:"shared"`
	} `yaml:"drivers"`
	Bootstrap struct {
		Doc     string   `yaml:"doc"`
		Surface []string `yaml:"surface"`
	} `yaml:"bootstrap"`
	SchemaDoc []Pair `yaml:"schema_doc"`
	Seams     []Seam `yaml:"seams"`
	Surface   struct {
		Dir  string   `yaml:"dir"`
		Docs []string `yaml:"docs"`
	} `yaml:"surface"`
}

// ParseConfig parses rules.yaml, rejecting unknown keys so a typo is not a
// silently empty check.
func ParseConfig(data []byte) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("rules.yaml: %w", err)
	}
	return &c, nil
}

// Checks are the tier-1 check IDs a rules.yaml check entry may name.
var Checks = []string{
	"rule-index", "design-front-matter", "drivers-claim", "bootstrap-surface",
	"schema-doc", "seam-contract", "surface-docs", "surface-removed",
	"ledger-single-writer", "relay-diskless", "executor-no-cgo", "large-literal",
	"exported-doc", "gofmt-touched", "error-wrap", "tests-with-change",
}

// SelfCheck is the rule-index check: the index's own problems, every
// automated rule mapped, every mapping naming a live rule and exactly one
// tier (LINT-01, LINT-02).
func SelfCheck(ix *Index, c *Config) []report.Finding {
	out := append([]report.Finding(nil), ix.Problems...)
	f := func(rule, format string, a ...any) {
		out = append(out, report.Finding{Rule: "LINT-02", Severity: ix.Severity("LINT-02"),
			File: "tools/tier1/rules.yaml", Check: "rule-index",
			Message: rule + ": " + fmt.Sprintf(format, a...)})
	}
	live := map[string]bool{}
	for _, r := range ix.Rules {
		live[r.ID] = true
		if _, ok := c.Rules[r.ID]; r.Automated && !ok {
			f(r.ID, "automated-tagged in %s but not mapped in rules.yaml", r.File)
		}
	}
	for _, id := range sortedKeys(c.Rules) {
		m := c.Rules[id]
		if !live[id] {
			f(id, "mapped in rules.yaml but no such rule in %s", Dir)
		}
		n := 0
		for _, set := range []bool{len(m.Check) > 0, m.Owner != "", m.FollowUp != ""} {
			if set {
				n++
			}
		}
		if n != 1 {
			f(id, "needs exactly one of check, owner or follow-up")
		}
		for _, ch := range m.Check {
			if !contains(Checks, ch) {
				f(id, "unknown check %q", ch)
			}
		}
	}
	return out
}

func sortedKeys(m map[string]Mapping) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
