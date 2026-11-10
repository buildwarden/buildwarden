package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ruled is design with one rule change declared, its review required.
var ruled = strings.Replace(strings.Replace(design, "design_review: skip",
	"design_review: required", 1), "rules: [ORCH-02]\n", "rules: [ORCH-02, TEST-09]\n"+
	"rule_changes:\n  - {rule: TEST-09, change: add, paths: [docs/development/testing.md]}\n",
	1) + "\n## Rule changes\n\nTEST-09 is new: ...\n"

// put writes files on feat/demo, then commits and pushes them.
func put(t *testing.T, work string, files map[string]string) {
	t.Helper()
	for p, text := range files {
		full := filepath.Join(work, filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", "change")
	git(t, work, "push", "-q", "origin", "feat/demo")
	git(t, work, "fetch", "-q", "origin")
}

// clearance runs phase 3 (a process item's x2) and returns only the FLOW-14 results' failures, and
// whether any FLOW-14 result was reported.
func clearance(t *testing.T, env Env, round string) (string, bool) {
	t.Helper()
	f := demo(3)
	f.Round, f.DesignReview = round, "required"
	if round == "process" {
		f.Kind, f.Phase = "process", 2
	}
	var got []Result
	for _, r := range Phase(env, f) {
		if r.Name == "rule changes cleared (FLOW-14)" {
			got = append(got, r)
		}
	}
	return failures(got), len(got) > 0
}

func TestRuleChangeProblems(t *testing.T) {
	text := strings.Replace(ruled, "  - {rule: TEST-09",
		"  - {rule: nope, change: edit, paths: []}\n"+
			"  - {rule: FLOW-14, change: amend, paths: [.claude/agents/wf-plan.md]}\n"+
			"  - {rule: TEST-09", 1)
	d, err := ParseDesign(text, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Front.RuleChanges) != 3 || d.Front.RuleChanges[2].Paths[0] !=
		"docs/development/testing.md" {
		t.Fatalf("rule_changes = %+v", d.Front.RuleChanges)
	}
	got := strings.Join(d.Front.Problems("demo"), "; ")
	for _, want := range []string{`entry 1: rule "nope"`, `change "edit"`,
		"entry 1: paths is empty",
		"FLOW-14: .claude/agents/wf-plan.md changes only through a process item"} {
		if !strings.Contains(got, want) {
			t.Errorf("problems %q lack %q", got, want)
		}
	}
	if strings.Contains(got, "TEST-09") {
		t.Errorf("a valid entry has problems: %q", got)
	}
	d, _ = ParseDesign(design, tmpl)
	if p := d.Front.Problems("demo"); len(p) != 0 {
		t.Errorf("a design without rule_changes: %v", p)
	}
}

func TestSkipRefusedWithRuleChanges(t *testing.T) {
	skip := strings.Replace(ruled, "design_review: required", "design_review: skip", 1)
	env := Env{Repo: repo(t, skip)}
	if got := failures(Phase(env, demo(1))); !strings.Contains(got, "1 rule_changes") {
		t.Errorf("skip with a rule change: %q", got)
	}
	env = Env{Repo: repo(t, strings.Replace(ruled, "TEST-09 is new: ...", "", 1))}
	f := demo(1)
	f.DesignReview = "required"
	if got := failures(Phase(env, f)); !strings.Contains(got, "section filled: Rule changes") {
		t.Errorf("rule change without its section: %q", got)
	}
}

// TestRuleChangeApprovalIsNotADesignApproval: the rule change's line in the
// Design review section does not count as the design's own approval.
func TestRuleChangeApprovalIsNotADesignApproval(t *testing.T) {
	d, err := ParseDesign(ruled+"\n## Design review\n\n- r1: rule change TEST-09\n"+
		"  approved at `abc1234`.\n", tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if got := approvals(d); len(got) != 0 {
		t.Fatalf("approvals = %v, want none", got)
	}
	m := ruleApprovedRe.FindAllStringSubmatch(d.Sections["Design review"], -1)
	if len(m) != 1 || m[0][1] != "TEST-09" || m[0][2] != "abc1234" {
		t.Fatalf("rule change approvals = %v", m)
	}
}

func TestRuleClearance(t *testing.T) {
	work := repo(t, ruled)
	env := Env{Repo: work}
	sha := commit(t, work, ruled+"\n<!-- as reviewed -->\n")
	review := "\n## Design review\n\n- r1: approved at `" + sha +
		"`;\n  rule change TEST-09 approved at `" + sha + "`.\n"
	commit(t, work, ruled+review)

	if got, ran := clearance(t, env, "feature"); !ran || got != "" {
		t.Fatalf("no guarded edit: ran %v, %q", ran, got)
	}
	put(t, work, map[string]string{"docs/development/testing.md": "### TEST-09\n",
		".github/workflows/ci.yml": "jobs: {build: {steps: [{run: make build}]}}\n",
		"relay/x.go":               "package relay\n"})
	if got, _ := clearance(t, env, "feature"); got != "" {
		t.Errorf("approved edit: %q", got)
	}

	put(t, work, map[string]string{"tools/tier1/rules.yaml": "rules: {}\n"})
	if got, _ := clearance(t, env, "feature"); !strings.Contains(got,
		"tools/tier1/rules.yaml is in no approved rule change") {
		t.Errorf("unlisted guarded edit: %q", got)
	}
	git(t, work, "rm", "-q", "tools/tier1/rules.yaml")

	late := strings.Replace(ruled, "rule_changes:\n", "rule_changes:\n"+
		"  - {rule: TEST-10, change: add, paths: [docs/development/global.md]}\n", 1)
	put(t, work, map[string]string{"docs/design/demo.md": late + review +
		"- r2: rule change TEST-10 approved at `" + sha + "`.\n",
		"docs/development/global.md": "### TEST-10\n"})
	got, _ := clearance(t, env, "feature")
	if !strings.Contains(got, "global.md is in no approved rule change") ||
		!strings.Contains(got, "TEST-10 approved at "+sha+" was not in rule_changes") {
		t.Errorf("entry added after the approval: %q", got)
	}
	if strings.Contains(got, "testing.md") {
		t.Errorf("the approved edit failed alongside: %q", got)
	}

	put(t, work, map[string]string{".claude/agents/wf-plan.md": "x\n",
		"tools/workflow/queue/q.go": "package queue\n",
		".github/workflows/ci.yml":  "jobs: {build: {steps: [{run: ./dist/tier1 -base x}]}}\n"})
	got, _ = clearance(t, env, "feature")
	for _, want := range []string{
		".claude/agents/wf-plan.md changes only through a process item",
		"tools/workflow/queue/q.go changes only through a process item",
		".github/workflows/ci.yml is in no approved rule change"} {
		if !strings.Contains(got, want) {
			t.Errorf("machinery and CI edits: %q lack %q", got, want)
		}
	}

	if got, ran := clearance(t, env, "process"); !ran || got != "" {
		t.Errorf("process item's branch: ran %v, %q", ran, got)
	}
}

// TestRuleClearanceWorkflowJobs: a workflow edit is a rule change only when
// it changes a rule-running job or drops a needs: on one
// (rule-running-line-match). The incident, a job added to ci-ok's needs:,
// clears with no rule_changes.
func TestRuleClearanceWorkflowJobs(t *testing.T) {
	work := repo(t, design)
	env := Env{Repo: work}
	git(t, work, "checkout", "-q", "main")
	_ = os.MkdirAll(filepath.Join(work, ".github", "workflows"), 0o755)
	if err := os.WriteFile(filepath.Join(work, ".github", "workflows", "main.yml"),
		[]byte(mainYML), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", "ci")
	git(t, work, "push", "-q", "origin", "main")
	git(t, work, "checkout", "-q", "feat/demo")
	git(t, work, "merge", "-q", "--no-edit", "main")

	incident := edit(t, mainYML, "  ci-ok:\n", "  keytool:\n    runs-on: ubuntu-latest\n"+
		"    steps:\n      - run: go test ./cmd/warden-io -run Keytool\n  ci-ok:\n",
		"[changes, lint, tier1, tier1-test]", "[changes, lint, tier1, tier1-test, keytool]")
	put(t, work, map[string]string{".github/workflows/main.yml": incident})
	if got, ran := clearance(t, env, "feature"); !ran || got != "" {
		t.Errorf("the incident: ran %v, %q", ran, got)
	}

	put(t, work, map[string]string{".github/workflows/main.yml": edit(t, incident,
		"      - run: go test ./cmd/warden-io", "      - run: ./dist/tier1 -base x\n"+
			"      - run: go test ./cmd/warden-io")})
	if got, _ := clearance(t, env, "feature"); !strings.Contains(got,
		".github/workflows/main.yml is in no approved rule change") {
		t.Errorf("a tier1 step added: %q", got)
	}

	put(t, work, map[string]string{".github/workflows/main.yml": "jobs: [\n"})
	if got, _ := clearance(t, env, "feature"); !strings.Contains(got,
		".github/workflows/main.yml: branch: ") {
		t.Errorf("an unparseable workflow: %q", got)
	}
}

// TestDrops: a branch whose tier 1 no longer reports a finding main's does
// passes only when an approved rule change names the finding's rule
// (FLOW-14, Decision 6 of design-approved-rule-changes).
func TestDrops(t *testing.T) {
	work := repo(t, ruled)
	sha := commit(t, work, ruled+"\n<!-- as reviewed -->\n")
	commit(t, work, ruled+"\n## Design review\n\n- r1: approved at `"+sha+
		"`; rule change TEST-09 approved at `"+sha+"`.\n")
	report := func(rules ...string) []byte {
		var fs []string
		for _, r := range rules {
			fs = append(fs, `{"rule":"`+r+`","severity":"MUST","file":"a.go",`+
				`"line":3,"check":"c","message":"m"}`)
		}
		return []byte(`{"findings":[` + strings.Join(fs, ",") + `]}`)
	}
	drops := func(main, branch []byte) []Result {
		return Drops(work, "origin/main", "feat/demo", main, branch)
	}
	if rs := drops(report("GLOBAL-01"), report("GLOBAL-01", "TEST-09")); !OK(rs) {
		t.Errorf("nothing dropped: %s", failures(rs))
	}
	if rs := drops(report("TEST-09", "GLOBAL-01"), report("GLOBAL-01")); !OK(rs) {
		t.Errorf("dropped a finding the approved change names: %s", failures(rs))
	}
	got := failures(drops(report("GLOBAL-01"), report()))
	if !strings.Contains(got, "no approved rule change names GLOBAL-01") {
		t.Errorf("dropped a finding no change names: %q", got)
	}
	if rs := drops([]byte("{"), report()); OK(rs) {
		t.Error("an unreadable report passed")
	}
}
