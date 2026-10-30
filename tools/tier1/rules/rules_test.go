package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/tier1"
)

// TestRealIndex runs the self-check against the real docs/development/ and
// rules.yaml: both must stay clean, so a new automated rule fails here first.
func TestRealIndex(t *testing.T) {
	ix, err := Load(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Rules) < 50 {
		t.Fatalf("parsed %d rules; the header format changed?", len(ix.Rules))
	}
	c, err := ParseConfig(tier1.RulesYAML)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range SelfCheck(ix, c) {
		t.Errorf("%s:%d %s", f.File, f.Line, f.Message)
	}
	if got := ix.Severity("BOOT-07"); got != "MUST-review" {
		t.Errorf("BOOT-07 severity %q", got)
	}
}

const doc = `# Things

### AAA-01 — Fine

**Severity:** MUST
**Check:** automated (Arm A) + agent-judgment (Arm B)

---

### AAA-02 — No severity

**Check:** agent-judgment

### AAA-03 — Bad check

**Severity:** SHOULD
**Check:** automatic

### AAA-04 — Judgment only

**Severity:** MUST-review
**Check:** agent-judgment
`

func TestSelfCheckFixtures(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(Dir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := "### ZZZ-01 — Format example\n\n**Severity:** MUST\n**Check:** automated\n"
	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	ix, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Rules) != 4 {
		t.Fatalf("rules %+v; README.md must be skipped", ix.Rules)
	}
	c, err := ParseConfig([]byte("rules:\n  AAA-04: {owner: x}\n" +
		"  GONE-01: {follow-up: y}\n  AAA-02: {check: [nope]}\n" +
		"  AAA-03: {owner: x, follow-up: y}\n"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range SelfCheck(ix, c) {
		got = append(got, f.Message)
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{
		"AAA-02: no **Severity:** line",
		`AAA-03: check "automatic"`,
		"AAA-01: automated-tagged",
		"GONE-01: mapped in rules.yaml but no such rule",
		`AAA-02: unknown check "nope"`,
		"AAA-03: needs exactly one",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	if len(got) != 6 {
		t.Errorf("%d findings, want 6:\n%s", len(got), all)
	}
}

func TestUnknownKey(t *testing.T) {
	if _, err := ParseConfig([]byte("rulez: {}\n")); err == nil {
		t.Error("an unknown rules.yaml key parsed")
	}
}
