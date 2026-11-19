package check

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// TestProfileFileHere parses this repository's own profile file: it is the
// file wf reads for buildwarden/buildwarden, and its values are the ones
// the code fixed before (PROFILE-01).
func TestProfileFileHere(t *testing.T) {
	p, err := ReadProfile(filepath.Join("..", "..", "..", "docs", "development",
		"loop-profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Trunk != "main" || p.Branch != "feat/{item}" ||
		p.Designs.Template != "docs/design/TEMPLATE.md" {
		t.Errorf("trunk %q, branch %q, template %q", p.Trunk, p.Branch, p.Designs.Template)
	}
	if !reflect.DeepEqual(p.FastGate, [][]string{{"make", "lint"}, {"make", "test"}}) {
		t.Errorf("fast_gate %v", p.FastGate)
	}
	if !reflect.DeepEqual(p.Impact.Values["security_boundary"],
		[]string{"none", "host-env", "env-relay", "env-beyond"}) {
		t.Errorf("security_boundary values %v", p.Impact.Values)
	}
	// The pattern wf hard-coded before it read the profile (FLOW-14).
	const was = `tier1|golangci-lint|tools/workflow|\bmake\s+(lint|enforcement)\b`
	if p.Rules.RuleJobs != was {
		t.Errorf("rule_jobs %q, was %q", p.Rules.RuleJobs, was)
	}
	for _, path := range []string{"docs/development/process.md", "tools/tier1/x.go",
		"tools/workflow/check/x.go", "docs/design/TEMPLATE.md"} {
		if !hasPrefixIn(path, p.Rules.Paths) {
			t.Errorf("%s is not under rules.paths %v", path, p.Rules.Paths)
		}
	}
	if got := p.Native.Prepare["windows-hyperv"]; len(got) != 1 ||
		got[0][0] != "tools/relay-vm/hyperv/refresh-cache.sh" {
		t.Errorf("native.prepare %v", p.Native.Prepare)
	}
	if err := p.CheckTrunk("loop-profile.yaml", "main"); err != nil {
		t.Error(err)
	}
}

// hereProfile is this repository's profile file.
func hereProfile(t *testing.T) *Profile {
	t.Helper()
	p, err := ReadProfile(filepath.Join("..", "..", "..", filepath.FromSlash(profilePath)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// hereRuns is this repository's rule_jobs pattern.
func hereRuns(t *testing.T) *regexp.Regexp {
	t.Helper()
	return regexp.MustCompile(hereProfile(t).Rules.RuleJobs)
}

func hasPrefixIn(p string, prefixes []string) bool {
	for _, pre := range prefixes {
		if strings.HasPrefix(p, pre) || p == pre {
			return true
		}
	}
	return false
}

func TestParseProfile(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
	}{
		{"minimal", "version: 1\n", ""},
		{"empty", "", "is empty"},
		{"unknown version", "version: 2\n", "version 2 is not one this wf reads"},
		{"no version", "component: x\n", "version 0 is not one this wf reads"},
		{"unknown key", "version: 1\nfast_gates: [[make]]\n",
			"field fast_gates not found"},
		{"not yaml", "version: [\n", "p.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ParseProfile("p.yaml", []byte(tc.text))
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				// An absent key is the term none.
				if p.FastGate != nil || p.Rules.Paths != nil ||
					p.Impact.Keys != nil || p.ReferenceDocs.Guides != nil ||
					p.Native.Prepare != nil || p.Rules.Index != "" {
					t.Errorf("absent keys not none: %+v", p)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) ||
				!strings.Contains(err.Error(), "p.yaml") {
				t.Errorf("err %v, want %q naming p.yaml", err, tc.want)
			}
		})
	}
}

func TestProfileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.yaml")
	text := "version: 1\ncomponent: relay\ntrunk: main\nfast_gate:\n  - [cargo, test]\n" +
		"reference_docs:\n  guides: [book/]\nrules:\n  paths: [rules/]\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := ReadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Component != "relay" ||
		!reflect.DeepEqual(p.FastGate, [][]string{{"cargo", "test"}}) ||
		!reflect.DeepEqual(p.ReferenceDocs.Guides, []string{"book/"}) ||
		!reflect.DeepEqual(p.Rules.Paths, []string{"rules/"}) || p.Rules.Machinery != nil {
		t.Errorf("%+v", p)
	}
	if _, err := ReadProfile(filepath.Join(dir, "absent.yaml")); err == nil {
		t.Error("absent file read")
	}
}

func TestProfileCheckTrunk(t *testing.T) {
	p := &Profile{Trunk: "trunk"}
	err := p.CheckTrunk("p.yaml", "main")
	if err == nil || !strings.Contains(err.Error(), `"trunk"`) ||
		!strings.Contains(err.Error(), `"main"`) ||
		!strings.Contains(err.Error(), "p.yaml") {
		t.Errorf("err %v, want both values and the file", err)
	}
	if err := (&Profile{}).CheckTrunk("p.yaml", "main"); err != nil {
		t.Errorf("absent trunk: %v", err)
	}
}
