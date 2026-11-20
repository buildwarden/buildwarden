package check

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The rule severities (LINT-01).
const (
	Must       = "MUST"
	MustReview = "MUST-review"
	Should     = "SHOULD"
	Hint       = "hint"
)

// IndexedRule is one rule header of a component's rule files with its
// metadata lines (LINT-01).
type IndexedRule struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Severity  string `json:"severity"`
	Check     string `json:"check"`
	Automated bool   `json:"automated"`
	File      string `json:"file"`
	Line      int    `json:"line"`
}

// RuleIndex is the rule index extracted from a component's rule directory,
// the profile's rules.dir (LINT-01).
type RuleIndex struct {
	Rules []IndexedRule
	// Problems are headers whose Severity or Check line is missing or invalid.
	Problems []Finding
}

// Severity is the rule's severity, or MUST for an ID the index lacks, so an
// unknown rule never silently stops blocking.
func (ix *RuleIndex) Severity(id string) string {
	for _, r := range ix.Rules {
		if r.ID == id && r.Severity != "" {
			return r.Severity
		}
	}
	return Must
}

var (
	headerRe = regexp.MustCompile(`^### ([A-Z]+-\d{2}) — (.+)$`)
	sevRe    = regexp.MustCompile(`^\*\*Severity:\*\*\s*(.*)$`)
	checkRe  = regexp.MustCompile(`^\*\*Check:\*\*\s*(.*)$`)
	qualRe   = regexp.MustCompile(`\s*\([^)]*\)$`)
)

// LoadRuleIndex parses every dir/*.md under root except README.md, whose
// header is the format example.
func LoadRuleIndex(root, dir string) (*RuleIndex, error) {
	files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	ix := &RuleIndex{}
	for _, f := range files {
		if filepath.Base(f) == "README.md" {
			continue
		}
		text, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		ix.parse(path.Join(dir, filepath.Base(f)), string(text))
	}
	return ix, nil
}

func (ix *RuleIndex) parse(file, text string) {
	var cur *IndexedRule
	finish := func() {
		if cur != nil {
			ix.validate(*cur)
			ix.Rules = append(ix.Rules, *cur)
		}
		cur = nil
	}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if m := headerRe.FindStringSubmatch(line); m != nil {
			finish()
			cur = &IndexedRule{ID: m[1], Title: m[2], File: file, Line: i + 1}
			continue
		}
		if cur == nil {
			continue
		}
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			finish()
		} else if m := sevRe.FindStringSubmatch(line); m != nil && cur.Severity == "" {
			cur.Severity = strings.TrimSpace(m[1])
		} else if m := checkRe.FindStringSubmatch(line); m != nil && cur.Check == "" {
			cur.Check = strings.TrimSpace(m[1])
			cur.Automated = strings.Contains(cur.Check, "automated")
		}
	}
	finish()
}

func (ix *RuleIndex) validate(r IndexedRule) {
	bad := func(format string, a ...any) {
		ix.Problems = append(ix.Problems, Finding{Rule: "LINT-01", Severity: Must,
			File: r.File, Line: r.Line, Check: "rule-index",
			Message: r.ID + ": " + fmt.Sprintf(format, a...)})
	}
	switch r.Severity {
	case Must, MustReview, Should:
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

// RuleMapping is one entry of the rule-to-check map. Exactly one field is
// set.
type RuleMapping struct {
	Check    []string `yaml:"check"`
	Owner    string   `yaml:"owner"`
	FollowUp string   `yaml:"follow-up"`
}

// RuleMap is the loop's part of a component's rule-to-check map, the
// profile's rules.index (here tools/tier1/rules.yaml): the mapping of each
// rule, and the designs whose front-matter problems are listed rather than
// failed (FLOW-12).
type RuleMap struct {
	Rules map[string]RuleMapping `yaml:"rules"`
	// DesignTolerate maps a design path to the front-matter keys whose FLOW-02
	// problems are listed for it rather than failed: designs kept as merged
	// although they predate a key (FLOW-12).
	DesignTolerate map[string][]string `yaml:"design_tolerate"`
}

// ParseRuleMap reads the loop's keys of a rule-to-check map. The file may
// hold a component's own keys besides (tier 1's here), which its own parse
// checks, so other keys are not an error here.
func ParseRuleMap(name string, data []byte) (*RuleMap, error) {
	var m RuleMap
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &m, nil
}

// SelfCheck is the rule-index check: the index's own problems, every
// automated rule mapped, every mapping naming a live rule and exactly one
// tier (LINT-01, LINT-02). file is the map's path, dir the rules' directory;
// checks, when not nil, are the check IDs a mapping may name.
func SelfCheck(ix *RuleIndex, m *RuleMap, file, dir string, checks []string) []Finding {
	out := append([]Finding(nil), ix.Problems...)
	f := func(rule, format string, a ...any) {
		out = append(out, Finding{Rule: "LINT-02", Severity: ix.Severity("LINT-02"),
			File: file, Check: "rule-index",
			Message: rule + ": " + fmt.Sprintf(format, a...)})
	}
	live := map[string]bool{}
	for _, r := range ix.Rules {
		live[r.ID] = true
		if _, ok := m.Rules[r.ID]; r.Automated && !ok {
			f(r.ID, "automated-tagged in %s but not mapped in rules.yaml", r.File)
		}
	}
	for _, id := range sortedKeys(m.Rules) {
		r := m.Rules[id]
		if !live[id] {
			f(id, "mapped in rules.yaml but no such rule in %s", dir)
		}
		n := 0
		for _, set := range []bool{len(r.Check) > 0, r.Owner != "", r.FollowUp != ""} {
			if set {
				n++
			}
		}
		if n != 1 {
			f(id, "needs exactly one of check, owner or follow-up")
		}
		for _, ch := range r.Check {
			if checks != nil && !oneOf(ch, checks...) {
				f(id, "unknown check %q", ch)
			}
		}
	}
	return out
}
