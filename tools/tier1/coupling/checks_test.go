package coupling

import (
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/tier1"
	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/internal/gittest"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
	"github.com/buildwarden/buildwarden/tools/tier1/rules"
)

func d(drivers []string, rs ...string) string { return design("f", drivers, nil, rs) }

const pkg = "package x\n"

func TestCouplingChecks(t *testing.T) {
	cases := []struct {
		name   string
		change map[string]string
		check  func(*Checker) []report.Finding
		want   []string // a substring of each finding's message, in order
	}{
		{"driver claimed", map[string]string{"docs/design/f.md": d([]string{"qemu"}),
			"driver/qemu/x.go": pkg}, (*Checker).DriversClaim, nil},
		{"driver not claimed", map[string]string{"docs/design/f.md": d([]string{"qemu"}),
			"driver/vz/x.go": pkg}, (*Checker).DriversClaim,
			[]string{"reaches driver/vz/"}},
		{"shared with too few drivers", map[string]string{
			"docs/design/f.md": d([]string{"qemu", "vz"}), "relay/x.go": pkg},
			(*Checker).DriversClaim, []string{"must also declare [container hyperv]"}},
		{"shared with every driver", map[string]string{
			"docs/design/f.md":  d([]string{"container", "qemu", "vz", "hyperv"}),
			"driver/session.go": pkg}, (*Checker).DriversClaim, nil},
		{"shared under interface", map[string]string{
			"docs/design/f.md": d([]string{"interface"}), "driver/vz/x.go": pkg,
			"ledger/x.go": pkg}, (*Checker).DriversClaim, nil},
		{"surface without bootstrap.md", map[string]string{"docs/design/f.md": d(nil),
			"cmd/warden-io/trust_linux.go": pkg}, (*Checker).BootstrapSurface,
			[]string{"is on the bootstrap surface"}},
		{"surface with bootstrap.md", map[string]string{
			"docs/design/f.md":              d(nil, "BOOT-07"),
			"docs/development/bootstrap.md": "m\n", "bootstrap/x.go": pkg},
			(*Checker).BootstrapSurface, nil},
		{"bootstrap.md with no design citing BOOT-07", map[string]string{
			"docs/design/f.md":              d(nil, "BOOT-01"),
			"docs/development/bootstrap.md": "m\n"},
			(*Checker).BootstrapSurface, []string{"no design in the diff cites"}},
		{"schema without spec", map[string]string{"schemas/v1/a.json": "{}\n"},
			(*Checker).SchemaDoc, []string{"schemas/v1/a.json changed without"}},
		{"spec without schema", map[string]string{
			"docs/book/src/design/ledger-spec.md": "s\n"},
			(*Checker).SchemaDoc, []string{"with nothing under schemas/"}},
		{"schema with spec", map[string]string{"schemas/v1/a.json": "{}\n",
			"docs/book/src/design/ledger-spec.md": "s\n"}, (*Checker).SchemaDoc, nil},
		{"test changed with code", map[string]string{"relay/x.go": pkg,
			"relay/x_test.go": pkg}, (*Checker).TestsWithChange, nil},
		{"code without test", map[string]string{"relay/x.go": pkg, "relay/a/y.go": pkg,
			"relay/a/testdata/z.go": pkg, "relay/a/y_test.go": pkg},
			(*Checker).TestsWithChange, []string{"changed in relay/ and none"}},
	}
	for _, tc := range cases {
		got := tc.check(checker(t, tc.change))
		if len(got) != len(tc.want) {
			t.Errorf("%s: %d findings, want %d: %+v",
				tc.name, len(got), len(tc.want), got)
			continue
		}
		for i, w := range tc.want {
			if !strings.Contains(got[i].Message, w) {
				t.Errorf("%s: %q lacks %q", tc.name, got[i].Message, w)
			}
		}
	}
}

func TestSeverities(t *testing.T) {
	c := checker(t, map[string]string{"docs/design/f.md": d(nil),
		"cmd/warden-io/jks.go": pkg, "relay/x.go": pkg})
	for _, f := range c.All() {
		want := map[string]report.Severity{"BOOT-07": report.Hint,
			"ORCH-01": report.Must, "GLOBAL-04": report.Hint}[f.Rule]
		if f.Severity != want {
			t.Errorf("%s: severity %q, want %q", f.Rule, f.Severity, want)
		}
	}
}

func TestBootstrapDocSeverity(t *testing.T) {
	c := checker(t, map[string]string{"docs/design/f.md": d(nil),
		"docs/development/bootstrap.md": "m\n"})
	got := c.BootstrapSurface()
	if len(got) != 1 || got[0].Severity != report.MustReview {
		t.Errorf("bootstrap.md with no BOOT-07 design must stay MUST-review: %+v", got)
	}
}

func TestCommentOnlyChange(t *testing.T) {
	r := gittest.New(t, map[string]string{"docs/design/TEMPLATE.md": tmpl,
		"driver/fixable.go": "package driver\n\n// Old says.\nfunc F() {}\n",
		"driver/vz/a.go":    "package vz\n\nfunc G() {}\n"})
	r.Commit("comments", map[string]string{"docs/design/f.md": d(nil),
		"driver/fixable.go": "package driver\n\n// New says, at length.\nfunc F() {\n}\n",
		"driver/vz/a.go":    "package vz\n\nfunc G() { _ = 1 }\n"})
	dd, err := diff.Load(r.Dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := rules.ParseConfig(tier1.RulesYAML)
	c := &Checker{Diff: dd, Cfg: cfg, Index: &rules.Index{}, Designs: LoadDesigns(dd, nil)}
	got := c.DriversClaim()
	if len(got) != 1 || got[0].File != "driver/vz/a.go" {
		t.Errorf("a comment-only change reached the shared package: %+v", got)
	}
}

func TestSeamContract(t *testing.T) {
	c := checker(t, map[string]string{"docs/design/f.md": d(nil), "relaycfg/x.go": pkg})
	if got := c.SeamContract(); len(got) != 0 {
		t.Errorf("no seam has a contract document yet: %+v", got)
	}
	c.Cfg.Seams = []rules.Seam{{Name: "claim", Code: []string{"relaycfg/"},
		Contract: "docs/claim.md"}}
	got := c.SeamContract()
	if len(got) != 1 || got[0].Rule != "GLOBAL-08" || got[0].File != "relaycfg/x.go" {
		t.Errorf("seam change without its contract: %+v", got)
	}
}
