package golangci

import (
	"testing"

	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

const canned = `{"Issues":[
 {"FromLinter":"revive","Text":"exported: exported function Foo should have comment",
  "Pos":{"Filename":"relay/a.go","Line":3,"Column":1}},
 {"FromLinter":"gofmt","Text":"File is not properly formatted",
  "Pos":{"Filename":"relay/b.go","Line":7}},
 {"FromLinter":"errorlint","Text":"non-wrapping format verb for fmt.Errorf",
  "Pos":{"Filename":"driver/c.go","Line":9}},
 {"FromLinter":"forbidigo","Text":"use of panic forbidden",
  "Pos":{"Filename":"driver/d.go","Line":2}}
],"Report":{}}`

func TestParse(t *testing.T) {
	sev := map[string]report.Severity{"GLOBAL-01": report.Must, "GLOBAL-02": report.Should,
		"GLOBAL-03": report.Must}
	got, err := Parse([]byte(canned), func(r string) report.Severity { return sev[r] })
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		rule, check, file string
		line              int
		sev               report.Severity
	}{
		{"GLOBAL-01", "exported-doc", "relay/a.go", 3, report.Must},
		{"GLOBAL-02", "gofmt-touched", "relay/b.go", 7, report.Should},
		{"GLOBAL-03", "error-wrap", "driver/c.go", 9, report.Must},
		{"GLOBAL-03", "error-wrap", "driver/d.go", 2, report.Must},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, w := range want {
		g := got[i]
		if g.Rule != w.rule || g.Check != w.check || g.File != w.file ||
			g.Line != w.line || g.Severity != w.sev {
			t.Errorf("issue %d: %+v, want %+v", i, g, w)
		}
	}
}

func TestParseErrors(t *testing.T) {
	sev := func(string) report.Severity { return report.Must }
	if _, err := Parse([]byte(`{"Issues":[{"FromLinter":"lll"}]}`), sev); err == nil {
		t.Error("an unmapped linter parsed")
	}
	if _, err := Parse([]byte(`not json`), sev); err == nil {
		t.Error("bad JSON parsed")
	}
	if got, err := Parse([]byte(`{"Issues":null}`), sev); err != nil || len(got) != 0 {
		t.Errorf("no issues: %v %v", got, err)
	}
}
