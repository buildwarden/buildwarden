package analyzers

import (
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/internal/gittest"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

// TestRun drives the runner over a scratch module: only changed packages under
// a check's scope are analyzed, and findings carry repo-relative paths.
func TestRun(t *testing.T) {
	r := gittest.New(t, map[string]string{"go.mod": "module example.com/m\n\ngo 1.22\n",
		"relay/a.go": "package relay\n", "other/a.go": "package other\n"})
	const create = "\n\nimport \"os\"\n\nfunc f() { _, _ = os.Create(\"x\") }\n"
	r.Commit("change", map[string]string{
		"relay/a.go":      "package relay" + create,
		"relay/a_test.go": "package relay" + strings.Replace(create, "f()", "g()", 1),
		"other/a.go":      "package other" + create,
	})
	d, err := diff.Load(r.Dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Run(d, func(string) report.Severity { return report.Must })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].File != "relay/a.go" || got[0].Line != 5 ||
		got[0].Rule != "RELAY-07" {
		t.Errorf("findings %+v, want one RELAY-07 at relay/a.go:5", got)
	}
}
