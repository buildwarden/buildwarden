package diff

import (
	"errors"
	"reflect"
	"testing"

	"github.com/buildwarden/buildwarden/tools/tier1/internal/gittest"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

const five = "1\n2\n3\n4\n5\n"

func scratch(t *testing.T) *gittest.Repo {
	r := gittest.New(t, map[string]string{
		"keep.go": five, "edit.go": five, "gone.go": five,
		"old/name.go": "package old\n\nfunc Moved() {}\n",
	})
	r.Commit("change\n\nTier1-Allow: UX-01 cmd/warden/*: docs follow in the guide item\n"+
		"Tier1-Allow: RELAY-07 relay/\n", map[string]string{
		"edit.go": "1\n2\nthree\n4\n5\n6\n", "gone.go": "", "add.go": "a\nb\n",
	})
	r.Commit("rename", map[string]string{"old/name.go": "",
		"new/name.go": "package old\n\nfunc Moved() {}\n"})
	return r
}

func TestLoad(t *testing.T) {
	r := scratch(t)
	d, err := Load(r.Dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]File{}
	for _, f := range d.Files {
		got[f.Path] = f
	}
	want := map[string]File{
		"add.go":      {Status: 'A', Path: "add.go"},
		"edit.go":     {Status: 'M', Path: "edit.go"},
		"gone.go":     {Status: 'D', Path: "gone.go"},
		"new/name.go": {Status: 'R', Path: "new/name.go", Old: "old/name.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("files:\n got %+v\nwant %+v", got, want)
	}
	for _, p := range []string{"add.go", "edit.go", "gone.go", "old/name.go", "new/name.go"} {
		if !d.Touched(p) {
			t.Errorf("Touched(%q) false", p)
		}
	}
	if d.Touched("keep.go") {
		t.Error("Touched(keep.go) true")
	}
	lines := []struct {
		path string
		line int
		want bool
	}{
		{"edit.go", 2, false}, {"edit.go", 3, true}, {"edit.go", 6, true},
		{"edit.go", 5, false}, {"add.go", 1, true}, {"add.go", 2, true},
		{"keep.go", 1, false}, {"new/name.go", 1, false},
	}
	for _, c := range lines {
		if got := d.LineChanged(c.path, c.line); got != c.want {
			t.Errorf("LineChanged(%s, %d) = %v, want %v", c.path, c.line, got, c.want)
		}
	}
}

func TestTrailers(t *testing.T) {
	r := scratch(t)
	d, err := Load(r.Dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	want := Allow{Rule: "UX-01", Glob: "cmd/warden/*", Reason: "docs follow in the guide item"}
	if len(d.Allows) == 1 {
		want.Commit = d.Allows[0].Commit
	}
	if len(d.Allows) != 1 || d.Allows[0] != want || want.Commit == "" {
		t.Errorf("allows: %+v", d.Allows)
	}
	if len(d.Bad) != 1 || d.Bad[0].Rule != "LINT-05" {
		t.Errorf("a trailer with no reason should be one LINT-05 finding: %+v", d.Bad)
	}
	// A trailer before the base is outside the range and does not apply.
	r.Git("push", "-q", "origin", "HEAD:main")
	r.Commit("later", map[string]string{"later.go": "x\n"})
	d, err = Load(r.Dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Allows) != 0 || len(d.Bad) != 0 {
		t.Errorf("trailers outside the range applied: %+v %+v", d.Allows, d.Bad)
	}
}

func TestSuppress(t *testing.T) {
	a := Allow{Rule: "UX-01", Glob: "cmd/warden/*", Reason: "why", Commit: "c"}
	d := &Diff{Allows: []Allow{a}}
	rep := &report.Report{}
	rep.Add(
		report.Finding{Rule: "UX-01", Severity: report.Must, File: "cmd/warden/main.go"},
		report.Finding{Rule: "UX-01", Severity: report.Must, File: "cmd/warden/x.go"},
		report.Finding{Rule: "UX-01", Severity: report.Must, File: "cmd/relay/main.go"},
		report.Finding{Rule: "UX-03", Severity: report.Must, File: "cmd/warden/main.go"},
	)
	d.Suppress(rep)
	if len(rep.Findings) != 2 || len(rep.Suppressed) != 1 ||
		rep.Suppressed[0].Reason != "why" {
		t.Errorf("findings %+v, suppressed %+v", rep.Findings, rep.Suppressed)
	}
}

func TestUnresolvableBase(t *testing.T) {
	r := scratch(t)
	_, err := Load(r.Dir, "origin/nope")
	var be *BaseError
	if !errors.As(err, &be) || be.Fix() != "git fetch origin nope" {
		t.Fatalf("err %v, want a BaseError naming git fetch", err)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"relay/", "relay/a/b.go", true},
		{"relay/", "relaycfg/a.go", false},
		{"driver/*.go", "driver/session.go", true},
		{"driver/*.go", "driver/qemu/x.go", false},
		{"cmd/**", "cmd/warden/x.go", true},
		{"cmd/warden-io/initialize*.go", "cmd/warden-io/initialize_test.go", true},
		{"a?c", "abc", true},
		{"a.c", "abc", false},
	}
	for _, c := range cases {
		if got := Match(c.glob, c.path); got != c.want {
			t.Errorf("Match(%q, %q) = %v", c.glob, c.path, got)
		}
	}
}
