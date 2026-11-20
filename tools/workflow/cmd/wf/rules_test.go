package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// loopFront is a design's front matter with only the loop's keys.
const loopFront = "---\nfeature: %s\nstatus: draft\nseams: []\nalters_relationships: false\n" +
	"design_review: skip\nnative_validation: []\nrules: [ABC-01]\nconcepts: [x]\n---\n\n# x\n"

// TestRulesCommand: wf rules runs the loop's generic checks on a component
// whose minimal profile file names no rule-to-check map and no impact keys:
// rule-index checks only the rule IDs, a design needs only the loop's keys,
// and with -base the touched designs and the trailers are checked
// (Validation plan test 12, a13 (b)).
func TestRulesCommand(t *testing.T) {
	repo := codeRepo(t)
	files := map[string]string{
		"loop-profile.yaml": "version: 1\nrules:\n  dir: rules\n" +
			"designs:\n  dir: designs\n  template: designs/TEMPLATE.md\n",
		"rules/a.md": "### ABC-01 — One\n\n**Severity:** MUST\n**Check:** automated\n\n" +
			"### ABC-02 — Two\n\n**Check:** agent-judgment\n",
		"designs/TEMPLATE.md": "---\nfeature: x\n---\n",
		"designs/good.md":     strings.Replace(loopFront, "%s", "good", 1),
	}
	for p, text := range files {
		_ = os.MkdirAll(filepath.Join(repo, filepath.Dir(p)), 0o755)
		write(t, repo, p, text)
	}
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "component")
	rules := func(args ...string) (string, error) {
		return wfAt(t, t.TempDir(), time.Now(), "rules", append([]string{"-repo", repo,
			"-profile", "loop-profile.yaml"}, args...)...)
	}
	out, err := rules()
	if err == nil || !strings.Contains(out, "MUST\tLINT-01\trule-index\trules/a.md:6\t"+
		"ABC-02: no **Severity:** line") || strings.Contains(out, "good.md") {
		t.Fatalf("wf rules: err %v\n%s", err, out)
	}
	write(t, repo, "rules/a.md", "### ABC-01 — One\n\n**Severity:** MUST\n"+
		"**Check:** automated\n")
	gitT(t, repo, "commit", "-q", "-am", "fix the index")
	if out, err := rules(); err != nil || !strings.HasSuffix(out, "0 findings, 0 MUST\n") {
		t.Fatalf("a clean component: err %v\n%s", err, out)
	}
	bad := strings.Replace(loopFront, "%s", "bad", 1)
	write(t, repo, "designs/bad.md", strings.Replace(bad, "concepts: [x]\n", "", 1))
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "a design\n\nTier1-Allow: no form")
	out, err = rules("-base", "HEAD~1")
	for _, w := range []string{"MUST\tFLOW-02\tdesign-front-matter\tdesigns/bad.md\t" +
		"front-matter: missing concepts", "MUST\tLINT-05\tsuppression\t\tcommit "} {
		if !strings.Contains(out, w) {
			t.Errorf("wf rules -base lacks %q:\n%s", w, out)
		}
	}
	if err == nil {
		t.Error("wf rules -base passed with MUST findings")
	}
	if _, err := wfAt(t, t.TempDir(), time.Now(), "rules", "-repo", repo); err == nil ||
		!strings.Contains(err.Error(), "rules needs -profile") {
		t.Errorf("no -profile: %v", err)
	}
}
