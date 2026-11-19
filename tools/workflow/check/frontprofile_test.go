package check

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// problemsBefore is Front.Problems as wf hard-coded it before it read the
// profile file: this repository's twelve keys and GLOBAL-06's values.
func problemsBefore(fm *Front, feature string) []string {
	var out []string
	for _, k := range keyOrder {
		if !fm.present[k] && (k != "design_review" || fm.Status != "archived") {
			out = append(out, "missing "+k)
		}
	}
	if fm.Feature != feature {
		out = append(out, fmt.Sprintf("feature %q, queue says %q", fm.Feature, feature))
	}
	if !oneOf(fm.Status, "draft", "active", "archived") {
		out = append(out, fmt.Sprintf("status %q", fm.Status))
	}
	if !oneOf(fm.SecurityBoundary, "none", "host-env", "env-relay", "env-beyond") {
		out = append(out, fmt.Sprintf("security_boundary %q", fm.SecurityBoundary))
	}
	if len(fm.Rules) == 0 {
		out = append(out, "rules is empty")
	}
	if len(fm.Concepts) == 0 {
		out = append(out, "concepts is empty")
	}
	for _, nv := range fm.NativeValidation {
		if nv.Host == "" || nv.Blocking == nil || nv.Reason == "" {
			out = append(out, fmt.Sprintf(
				"native_validation %+v needs host, blocking and reason", nv))
		}
	}
	return append(out, ruleChangesBefore(fm.RuleChanges)...)
}

// ruleChangesBefore is ruleChangeProblems with kindBefore.
func ruleChangesBefore(rcs []RuleChange) []string {
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
			if kindBefore(p) == machinery {
				out = append(out, fmt.Sprintf("rule_changes %s: %s changes only "+
					"through a process item (FLOW-14)", who, p))
			}
		}
	}
	return out
}

// TestProblemsHereAsBefore: with this repository's profile file, every
// design in docs/design/ gets the result it got before (Validation plan
// test 2).
func TestProblemsHereAsBefore(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "docs", "design")
	tmplText := readFile(t, filepath.Join(dir, "TEMPLATE.md"))
	names, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil || len(names) < 50 {
		t.Fatalf("%d designs, %v", len(names), err)
	}
	p := hereProfile(t)
	for _, name := range names {
		text := readFile(t, name)
		if filepath.Base(name) == "TEMPLATE.md" || !strings.HasPrefix(text, "---\n") {
			continue
		}
		d, err := ParseDesign(text, tmplText)
		if err != nil {
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(name), ".md")
		got, want := d.Front.Problems(stem, p), problemsBefore(&d.Front, stem)
		if !slices.Equal(got, want) {
			t.Errorf("%s: problems %q, were %q", stem, got, want)
		}
	}
}

// withoutKeys drops the lines of the keys from the text.
func withoutKeys(text string, keys ...string) string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		k, _, _ := strings.Cut(l, ":")
		if !slices.Contains(keys, k) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestProblemsMinimalProfile: a component whose profile adds no impact keys
// needs only the loop's keys (Validation plan test 12, a17).
func TestProblemsMinimalProfile(t *testing.T) {
	minimal, err := ParseProfile("p.yaml", []byte("version: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	text := withoutKeys(design, "drivers", "surfaces", "security_boundary", "tier2_gaps")
	d, err := ParseDesign(text, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Front.Problems("demo", minimal); len(got) != 0 {
		t.Errorf("no impact keys: %q", got)
	}
	if got := d.Front.Problems("demo", hereProfile(t)); !slices.Equal(got, []string{
		"missing drivers", "missing surfaces", "missing security_boundary",
		"missing tier2_gaps", `security_boundary ""`}) {
		t.Errorf("this repository's keys: %q", got)
	}
	d, err = ParseDesign(withoutKeys(text, "concepts"), tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Front.Problems("demo", minimal); !slices.Equal(got,
		[]string{"missing concepts", "concepts is empty"}) {
		t.Errorf("a loop key missing: %q", got)
	}
}

// TestProblemsRepos: merge_first and a prefixed area name repositories in
// repos after the home (Validation plan test 8).
func TestProblemsRepos(t *testing.T) {
	text := strings.Replace(design, "seams: []\n", "seams: []\nrepos: [o/here, o/specs]\n"+
		"merge_first: [o/specs, o/here, o/relay]\nareas: [relay, o/specs:schema, "+
		"o/relay:core]\n", 1)
	d, err := ParseDesign(text, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"merge_first o/here is not in repos after the home",
		"merge_first o/relay is not in repos after the home",
		"area o/relay:core names o/relay, which is not in repos after the home"}
	if got := d.Front.Problems("demo", hereProfile(t)); !slices.Equal(got, want) {
		t.Errorf("problems %q, want %q", got, want)
	}
}
