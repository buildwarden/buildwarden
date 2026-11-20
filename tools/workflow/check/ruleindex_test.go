package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRuleIndex: the index reads each rule header of the rule directory
// with its severity and check, reports a header whose lines are missing or
// wrong, and SelfCheck holds the map to the index (LINT-01, LINT-02).
func TestRuleIndex(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "rules")
	_ = os.MkdirAll(dir, 0o755)
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("README.md", "### ABC-99 — The example\n")
	write("a.md", "### ABC-01 — One\n\n**Severity:** MUST\n**Check:** automated\n\n"+
		"### ABC-02 — Two\n\n**Severity:** MAYBE\n**Check:** agent-judgment (review)\n")
	ix, err := LoadRuleIndex(root, "rules")
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Rules) != 2 || ix.Rules[0].ID != "ABC-01" || !ix.Rules[0].Automated ||
		ix.Severity("ABC-01") != Must || ix.Severity("ZZZ-01") != Must {
		t.Fatalf("rules %+v", ix.Rules)
	}
	if len(ix.Problems) != 1 || !strings.Contains(ix.Problems[0].Message,
		`ABC-02: severity "MAYBE" is not MUST, MUST-review or SHOULD`) {
		t.Fatalf("problems %+v", ix.Problems)
	}
	m, err := ParseRuleMap("map.yaml", []byte("rules:\n  ABC-02: {owner: x}\n"+
		"  ABC-03: {check: [c], owner: y}\nother: kept\n"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range SelfCheck(ix, m, "map.yaml", "rules", []string{"d"}) {
		got = append(got, f.Message)
	}
	want := []string{`ABC-02: severity "MAYBE" is not MUST, MUST-review or SHOULD`,
		"ABC-01: automated-tagged in rules/a.md but not mapped in rules.yaml",
		"ABC-03: mapped in rules.yaml but no such rule in rules",
		"ABC-03: needs exactly one of check, owner or follow-up",
		`ABC-03: unknown check "c"`}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("SelfCheck:\n%s\nwant\n%s", strings.Join(got, "\n"),
			strings.Join(want, "\n"))
	}
}
