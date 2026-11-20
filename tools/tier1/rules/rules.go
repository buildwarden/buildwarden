// Package rules extracts the rule index from docs/development/ (LINT-01),
// loads rules.yaml, and self-checks that every automated-tagged rule is
// mapped to one tier (LINT-02).
package rules

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/buildwarden/buildwarden/tools/tier1/report"
	"github.com/buildwarden/buildwarden/tools/workflow/check"
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

// Load parses every docs/development/*.md under root except README.md, whose
// RELAY-01 header is the format example. The parse is wf's
// (tools/workflow/check, LINT-01), which a component's CI runs as wf rules.
func Load(root string) (*Index, error) {
	cix, err := check.LoadRuleIndex(root, Dir)
	if err != nil {
		return nil, err
	}
	ix := &Index{Problems: toReport(cix.Problems)}
	for _, r := range cix.Rules {
		ix.Rules = append(ix.Rules, Rule{ID: r.ID, Title: r.Title,
			Severity: report.Severity(r.Severity), Check: r.Check,
			Automated: r.Automated,
			File:      r.File, Line: r.Line})
	}
	return ix, nil
}

// toReport converts wf's findings to tier 1's.
func toReport(fs []check.Finding) []report.Finding {
	var out []report.Finding
	for _, f := range fs {
		out = append(out, report.Finding{Rule: f.Rule,
			Severity: report.Severity(f.Severity),
			File:     f.File, Line: f.Line, Check: f.Check, Message: f.Message})
	}
	return out
}

// Mapping is one rules.yaml entry. Exactly one field is set.
type Mapping = check.RuleMapping

// Seam is a versioned seam with a contract document (GLOBAL-08).
type Seam struct {
	Name     string   `yaml:"name"`
	Code     []string `yaml:"code"`
	Contract string   `yaml:"contract"`
}

// SchemaDoc is RELAY-05's specs/ layout: under Root, each
// <dir>/v<N>.schema.json has a v<N>.md beside it. ProseOnly names the
// directories that hold documents only.
type SchemaDoc struct {
	Root      string   `yaml:"root"`
	ProseOnly []string `yaml:"prose_only"`
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
	SchemaDoc SchemaDoc `yaml:"schema_doc"`
	Seams     []Seam    `yaml:"seams"`
	Surface   struct {
		Dir  string   `yaml:"dir"`
		Docs []string `yaml:"docs"`
	} `yaml:"surface"`
	// DesignTolerate maps a design path to the front-matter keys whose FLOW-02
	// problems are listed for it rather than failed: designs kept as merged
	// although they predate a key (FLOW-12).
	DesignTolerate map[string][]string `yaml:"design_tolerate"`
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
// tier (LINT-01, LINT-02), run by wf's check with tier 1's check IDs.
func SelfCheck(ix *Index, c *Config) []report.Finding {
	cix := &check.RuleIndex{}
	for _, f := range ix.Problems {
		cix.Problems = append(cix.Problems, check.Finding{Rule: f.Rule,
			Severity: string(f.Severity), File: f.File, Line: f.Line, Check: f.Check,
			Message: f.Message})
	}
	for _, r := range ix.Rules {
		cix.Rules = append(cix.Rules, check.IndexedRule{ID: r.ID, Title: r.Title,
			Severity: string(r.Severity), Check: r.Check, Automated: r.Automated,
			File: r.File, Line: r.Line})
	}
	m := &check.RuleMap{Rules: c.Rules, DesignTolerate: c.DesignTolerate}
	return toReport(check.SelfCheck(cix, m, "tools/tier1/rules.yaml", Dir, Checks))
}
