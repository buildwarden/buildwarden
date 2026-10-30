package surface

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/internal/gittest"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

func TestExtract(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "cmd.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Extract(map[string]string{"cmd/warden/main.go": string(src)})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for id := range s.Names {
		got = append(got, id)
	}
	sort.Strings(got)
	want := []string{"flag driver", "flag network", "flag quiet",
		"key mode", "key relay", "key runtime"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("names %v, want %v", got, want)
	}
	if n := s.Names["flag quiet"]; n.Line != 20 || n.File != "cmd/warden/main.go" {
		t.Errorf("quiet at %s:%d", n.File, n.Line)
	}
}

const base = `package main

type Config struct {
	Runtime string ` + "`toml:\"runtime\"`" + `
}

func init() {
	root.Flags().Bool("json", false, "")
	root.Flags().Bool("old", false, "")
}
`

const (
	addNew = "func x() { root.Flags().Bool(\"new\", false, \"\") }\n"
	depOld = "func y() { root.Flags().MarkDeprecated(\"old\", \"gone in v2\") }\n"
	defOld = `root.Flags().Bool("old", false, "")`
)

func sev(string) report.Severity { return report.Must }

func TestCheck(t *testing.T) {
	cases := []struct {
		name string
		head string
		docs bool
		want []string // rule and a message substring
	}{
		{"no change", base + "\n// comment\n", false, nil},
		{"added flag, no docs", base + addNew, false,
			[]string{"UX-01 +flag new"}},
		{"added flag, docs", base + addNew, true, nil},
		{"removed flag", strings.Replace(base, defOld, "", 1), true,
			[]string{`UX-03 flag "old" was removed`}},
		{"deprecated flag kept", base + depOld, false, nil},
		{"renamed toml key", strings.Replace(base, `toml:"runtime"`, `toml:"engine"`, 1),
			false, []string{"UX-01 +key engine, -key runtime", `UX-03 key "runtime"`}},
	}
	for _, tc := range cases {
		r := gittest.New(t, map[string]string{"cmd/warden/main.go": base,
			"cmd/warden/main_test.go": "package main\n"})
		change := map[string]string{"cmd/warden/main.go": tc.head}
		if tc.docs {
			change["docs/book/src/guide/cli.md"] = "cli\n"
		}
		r.Commit("change", change)
		d, err := diff.Load(r.Dir, "origin/main")
		if err != nil {
			t.Fatal(err)
		}
		got, err := Check(d, "cmd/warden/", []string{"docs/book/src/guide/cli.md"}, sev)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: %+v", tc.name, got)
			continue
		}
		for i, w := range tc.want {
			rule, msg, _ := strings.Cut(w, " ")
			if got[i].Rule != rule || !strings.Contains(got[i].Message, msg) {
				t.Errorf("%s: %+v, want %s", tc.name, got[i], w)
			}
		}
	}
}
