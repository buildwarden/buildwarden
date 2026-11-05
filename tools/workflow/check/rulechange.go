package check

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// RuleChange is one rule_changes entry: a rule change the feature makes on
// its own branch once its design review approves it by name (FLOW-14).
type RuleChange struct {
	Rule   string   `yaml:"rule"`
	Change string   `yaml:"change"`
	Paths  []string `yaml:"paths"`
}

// workflowsDir holds the CI workflows. Only their lines that run the rule
// enforcement are a rule change; the rest is ordinary code.
const workflowsDir = ".github/workflows/"

// ruleRunningRe matches a workflow line that runs the rule enforcement:
// tier 1, golangci-lint or the workflow tools (FLOW-14).
var ruleRunningRe = regexp.MustCompile(`tier1|golangci-lint|tools/workflow`)

// pathKind says what FLOW-14 makes of a changed path.
type pathKind int

const (
	ordinary  pathKind = iota
	rulePath           // cleared by an approved rule change
	machinery          // the loop's own machinery: process items only
)

// kindOf classifies a path. A workflow file is ordinary here; its changed
// lines decide (ruleRunningRe).
func kindOf(p string) pathKind {
	switch {
	case strings.HasPrefix(p, "docs/development/"), strings.HasPrefix(p, "tools/tier1/"),
		strings.HasPrefix(p, "tools/workflow/check/"), p == "docs/design/TEMPLATE.md":
		return rulePath
	case strings.HasPrefix(p, ".claude/"), strings.HasPrefix(p, "tools/workflow/"):
		return machinery
	}
	return ordinary
}

var ruleIDRe = regexp.MustCompile(`^[A-Z]+-\d{2}$`)

// ruleChangeProblems is the rule_changes part of Front.Problems.
func ruleChangeProblems(rcs []RuleChange) []string {
	var out []string
	for i, rc := range rcs {
		who := rc.Rule
		if !ruleIDRe.MatchString(rc.Rule) {
			who = fmt.Sprintf("entry %d", i+1)
			out = append(out, fmt.Sprintf("rule_changes %s: rule %q is not a rule ID",
				who, rc.Rule))
		}
		if !oneOf(rc.Change, "add", "amend", "retire") {
			out = append(out, fmt.Sprintf("rule_changes %s: change %q is not "+
				"add, amend or retire", who, rc.Change))
		}
		if len(rc.Paths) == 0 {
			out = append(out, fmt.Sprintf("rule_changes %s: paths is empty", who))
		}
		for _, p := range rc.Paths {
			if kindOf(p) == machinery {
				out = append(out, fmt.Sprintf("rule_changes %s: %s changes only "+
					"through a process item (FLOW-14)", who, p))
			}
		}
	}
	return out
}

// ruleApprovedRe finds a rule change's approval in the Design review
// section: `rule change <ID> approved at <sha>` (FLOW-19). The phrase may
// wrap across lines.
var ruleApprovedRe = regexp.MustCompile(
	"(?i:rule\\s+change)\\s+`?([A-Z]+-\\d{2})`?\\s+(?i:approved\\s+at)\\s+`?([0-9a-f]{7,40})")

// cleared returns the files each approved rule change may edit, keyed by
// path, with the rule that clears it. An approval clears an entry only when
// the entry was in the front-matter at the approved commit and still is;
// the paths are those approved. why lists the approvals that clear nothing.
func (c *checker) cleared(d *Design) (paths map[string]string, why []string) {
	paths = map[string]string{}
	now := map[string]bool{}
	for _, rc := range d.Front.RuleChanges {
		now[rc.Rule] = true
	}
	for _, m := range ruleApprovedRe.FindAllStringSubmatch(d.Sections["Design review"], -1) {
		rule, sha := m[1], m[2]
		if !now[rule] {
			why = append(why, fmt.Sprintf(
				"%s approved at %s is no longer in rule_changes", rule, sha))
			continue
		}
		then, err := c.loadDesign(sha)
		if err != nil {
			why = append(why, fmt.Sprintf("%s approved at %s: %v", rule, sha, err))
			continue
		}
		found := false
		for _, rc := range then.Front.RuleChanges {
			if rc.Rule == rule {
				found = true
				for _, p := range rc.Paths {
					paths[p] = rule
				}
			}
		}
		if !found {
			why = append(why, fmt.Sprintf(
				"%s approved at %s was not in rule_changes there", rule, sha))
		}
	}
	return paths, why
}

// ruleChangeFiles returns the files the branch changes since its merge base
// with base that FLOW-14 guards, with their kind: every rule-change and
// machinery path, and each workflow file whose changed lines run the rule
// enforcement.
func (c *checker) ruleChangeFiles(base, branch string) (map[string]pathKind, error) {
	names, err := c.git("diff", "--name-only", "--no-renames", base+"..."+branch)
	if err != nil {
		return nil, err
	}
	out := map[string]pathKind{}
	for _, p := range strings.Fields(names) {
		if k := kindOf(p); k != ordinary {
			out[p] = k
		}
	}
	patch, err := c.git("diff", "-U0", "--no-renames", base+"..."+branch, "--", workflowsDir)
	if err != nil {
		return nil, err
	}
	var file string
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			file = ""
		case strings.HasPrefix(line, "+++ b/"):
			file = strings.TrimPrefix(line, "+++ b/")
		case strings.HasPrefix(line, "--- a/") && file == "":
			file = strings.TrimPrefix(line, "--- a/")
		case strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, "--- "):
		case file != "" && line != "" && strings.ContainsAny(line[:1], "+-") &&
			ruleRunningRe.MatchString(line):
			out[file] = rulePath
		}
	}
	return out, nil
}

// ruleClearance is FLOW-14's automated part, run at the Implement, Review and
// Publish done-when: every guarded file a feature branch changes is in the
// paths of an approved rule change, and the loop's machinery is untouched. A
// process item's branch is the other FLOW-14 route and is exempt.
func (c *checker) ruleClearance() {
	const name = "rule changes cleared (FLOW-14)"
	if c.f.Round == "process" {
		c.add(name, true, "process item: the FLOW-14 process route")
		return
	}
	files, err := c.ruleChangeFiles("origin/main", c.f.Branch)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	if len(files) == 0 {
		c.add(name, true, "the branch changes no rule or its enforcement")
		return
	}
	d, err := c.loadDesign(c.f.Branch)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	ok, why := c.cleared(d)
	var good []string
	for _, p := range sortedKeys(files) {
		switch {
		case files[p] == machinery:
			c.add(name, false, "%s changes only through a process item", p)
		case ok[p] == "":
			c.add(name, false, "%s is in no approved rule change%s", p, because(why))
		default:
			good = append(good, p+" ("+ok[p]+")")
		}
	}
	if len(good) > 0 {
		c.add(name, true, "cleared: %s", strings.Join(good, ", "))
	}
}

func because(why []string) string {
	if len(why) == 0 {
		return ""
	}
	return "; " + strings.Join(why, "; ")
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Finding is a tier 1 finding as tier1 -json reports it
// (tools/tier1/report.schema.json).
type Finding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Check    string `json:"check"`
	Message  string `json:"message"`
}

func (f Finding) key() string {
	return fmt.Sprintf("%s|%s|%s|%d", f.Rule, f.Check, f.File, f.Line)
}

// Dropped returns the findings in was that now does not report, matched by
// rule, check, file and line.
func Dropped(was, now []Finding) []Finding {
	have := map[string]bool{}
	for _, f := range now {
		have[f.key()] = true
	}
	var out []Finding
	for _, f := range was {
		if !have[f.key()] {
			out = append(out, f)
		}
	}
	return out
}

// Drops is FLOW-14's check of an enforcement change by main's enforcement:
// mainReport is tier1 built from the merge base and run against the branch,
// branchReport the branch's own tier1 on the same diff. Every finding the
// branch's tier1 drops must be on a rule that an approved rule_changes entry
// names, in a design the diff from base to head changes.
func Drops(repo, base, head string, mainReport, branchReport []byte) []Result {
	const name = "main's tier 1 findings kept (FLOW-14)"
	c := &checker{env: Env{Repo: repo}, f: &queue.Feature{}}
	var was, now struct {
		Findings []Finding `json:"findings"`
	}
	if err := json.Unmarshal(mainReport, &was); err != nil {
		c.add(name, false, "main's report: %v", err)
		return c.out
	}
	if err := json.Unmarshal(branchReport, &now); err != nil {
		c.add(name, false, "the branch's report: %v", err)
		return c.out
	}
	dropped := Dropped(was.Findings, now.Findings)
	if len(dropped) == 0 {
		c.add(name, true, "the branch's tier 1 reports every finding main's does (%d)",
			len(was.Findings))
		return c.out
	}
	named, why, err := c.approvedRules(base, head)
	if err != nil {
		c.add(name, false, "%v", err)
		return c.out
	}
	for _, f := range dropped {
		where := fmt.Sprintf("%s %s %s:%d: %s", f.Rule, f.Check, f.File, f.Line, f.Message)
		if named[f.Rule] {
			c.add(name, true, "dropped by approved rule change %s: %s", f.Rule, where)
		} else {
			c.add(name, false, "dropped, and no approved rule change names %s: %s%s",
				f.Rule, where, because(why))
		}
	}
	return c.out
}

// approvedRules returns the rules of the approved rule changes in each
// design the diff from base to head adds or modifies.
func (c *checker) approvedRules(base, head string) (map[string]bool, []string, error) {
	names, err := c.git("diff", "--name-only", "--no-renames", "--diff-filter=AM",
		base+"..."+head, "--", "docs/design/")
	if err != nil {
		return nil, nil, err
	}
	out := map[string]bool{}
	var why []string
	for _, p := range strings.Fields(names) {
		if !strings.HasSuffix(p, ".md") || p == "docs/design/TEMPLATE.md" {
			continue
		}
		c.f.Design = p
		d, err := c.loadDesign(head)
		if err != nil {
			why = append(why, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		paths, w := c.cleared(d)
		why = append(why, w...)
		for _, rule := range paths {
			out[rule] = true
		}
	}
	return out, why, nil
}
