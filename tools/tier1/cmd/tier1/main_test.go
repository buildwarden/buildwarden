package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/tier1/internal/gittest"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"-nope"},
		{"stray"},
		{"rules", "-nope"},
		{"designs", "-nope"},
		{"designs", "stray"},
		{"designs", "-repo", "/nonexistent"},
		{"designs", "-base", "no-such-rev"},
	} {
		var out, errb bytes.Buffer
		if got := run(args, &out, &errb); got != report.ExitUsage {
			t.Errorf("%v: exit %d, want %d (stderr %q)", args, got, report.ExitUsage,
				errb.String())
		}
	}
}

const front = "---\nfeature: %s\nstatus: archived\nround: parallel\ndrivers: []\n" +
	"seams: []\nsurfaces: []\nsecurity_boundary: none\nalters_relationships: false\n" +
	"native_validation: []\ntier2_gaps: []\nrules: [FLOW-02]\nconcepts: [x]\n---\n\n# t\n"

// TestDesignsAll covers tier1 designs -all: pre-standard designs and the
// archived designs rules.yaml tolerates are listed without failing (FLOW-12);
// a design with broken front-matter fails.
func TestDesignsAll(t *testing.T) {
	files := map[string]string{
		"docs/design/TEMPLATE.md":               "---\nfeature: \n---\n",
		"docs/design/ok.md":                     fmt.Sprintf(front, "ok"),
		"docs/design/vm-driver-release-plan.md": "# VM Driver Release Plan\n",
		"docs/design/unified-bootstrap.md": strings.Replace(
			fmt.Sprintf(front, "unified-bootstrap"),
			"alters_relationships: false\n", "", 1),
	}
	r := gittest.New(t, files)
	var out, errb bytes.Buffer
	all := []string{"designs", "-all", "-repo", r.Dir}
	if got := run(all, &out, &errb); got != report.ExitClean {
		t.Fatalf("exit %d, want 0\n%s%s", got, out.String(), errb.String())
	}
	for _, want := range []string{
		"ok            docs/design/ok.md",
		"pre-standard  docs/design/vm-driver-release-plan.md",
		"tolerated     docs/design/unified-bootstrap.md: missing alters_relationships",
		"0 failing, 1 pre-standard, 1 with tolerated problems",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}

	r.Commit("break", map[string]string{"docs/design/ok.md": "---\nfeature: ok\n---\n"})
	out.Reset()
	if got := run(all, &out, &errb); got != report.ExitFail {
		t.Fatalf("exit %d, want 1\n%s", got, out.String())
	}
	if !strings.Contains(out.String(), "FAIL          docs/design/ok.md: missing status") {
		t.Errorf("output:\n%s", out.String())
	}

	// Without -all only the designs the diff touches are checked.
	out.Reset()
	if got := run([]string{"designs", "-repo", r.Dir}, &out, &errb); got != report.ExitFail ||
		!strings.Contains(out.String(), "1 checked") {
		t.Errorf("exit %d\n%s", got, out.String())
	}
}
