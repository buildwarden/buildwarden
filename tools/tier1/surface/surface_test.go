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

// The flags and keys of drivercmd.Setting literals, elided ones in a
// []drivercmd.Setting included, and nothing else from a driver directory.
func TestExtractSettings(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "driver.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := extract(map[string]string{"driver/container/settings.go": string(src)}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(s), []string{"flag runtime", "flag slots", "key cli",
		"key image_min_grace", "key image_refresh_after", "key relay_image"}; !reflect.DeepEqual(got, want) {
		t.Errorf("names %v, want %v", got, want)
	}
	if n := s.Names["flag slots"]; n.Line != 18 {
		t.Errorf("slots at line %d", n.Line)
	}
	if len(s.Literals) != 0 {
		t.Errorf("literals read from a driver directory: %v", s.Literals)
	}
	kit := "package drivercmd\n\nvar x = []Setting{{Flag: \"in-kit\", Key: \"driver.k.in_kit\"}}\n"
	s, err = extract(map[string]string{"driver/drivercmd/x.go": kit}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(s), []string{"flag in-kit", "key in_kit"}; !reflect.DeepEqual(got, want) {
		t.Errorf("names in package drivercmd %v, want %v", got, want)
	}
}

func ids(s *Surface) []string {
	var out []string
	for id := range s.Names {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

const (
	cmdWithFlag = "package main\n\nfunc init() { root.Flags().String(\"runtime\", \"\", \"\") }\n"
	cmdBare     = "package main\n"
	drvBase     = "package container\n\nimport \"github.com/buildwarden/buildwarden/driver/drivercmd\"\n\n" +
		"var settings = []drivercmd.Setting{\n\t{Name: \"relay-image\", " +
		"Key: \"driver.container.relay_image\"},\n}\n"
	drvRuntime = "package container\n\nimport \"github.com/buildwarden/buildwarden/driver/drivercmd\"\n\n" +
		"var settings = []drivercmd.Setting{\n\t{Name: \"relay-image\", " +
		"Key: \"driver.container.relay_image\"},\n\t{Name: \"runtime\", Flag: \"runtime\"},\n}\n"
	drvAdded = "package container\n\nimport \"github.com/buildwarden/buildwarden/driver/drivercmd\"\n\n" +
		"var settings = []drivercmd.Setting{\n\t{Name: \"relay-image\", " +
		"Key: \"driver.container.relay_image\"},\n\t{Name: \"x\", Flag: \"extra\"},\n}\n"
	drvEmpty = "package container\n"
	// The relay image's key moved to a new name, with and without its old
	// name kept as an alias.
	drvMovedAlias = "package container\n\nimport \"github.com/buildwarden/buildwarden/driver/drivercmd\"\n\n" +
		"var settings = []drivercmd.Setting{\n\t{Name: \"relay-image\", " +
		"Key: \"driver.container.image\", Aliases: []string{\"runtime.relay_image\"}},\n}\n"
	drvCommon = "package container\n\nimport \"github.com/buildwarden/buildwarden/driver/drivercmd\"\n\n" +
		"var settings = []drivercmd.Setting{\n\t{Name: \"relay-image\", " +
		"Key: \"driver.container.relay_image\"},\n\t{Name: \"grace\", " +
		"Common: \"image_min_grace\"},\n}\n"
	drvMovedBare = "package container\n\nimport \"github.com/buildwarden/buildwarden/driver/drivercmd\"\n\n" +
		"var settings = []drivercmd.Setting{\n\t{Name: \"relay-image\", " +
		"Key: \"driver.container.image\"},\n}\n"
)

// The drivers' Setting literals are part of the surface at both revisions:
// a flag moved into one is not removed, one added is a change for the docs,
// one removed is a removal, and a change under driver/ alone runs the check.
func TestCheckDrivers(t *testing.T) {
	cases := []struct {
		name             string
		baseCmd, baseDrv string
		headCmd, headDrv string
		docs             bool
		want             []string
	}{
		{"flag moved into a driver", cmdWithFlag, drvBase, cmdBare, drvRuntime, false, nil},
		{"flag added in a driver, no docs", cmdBare, drvBase, cmdBare, drvAdded, false,
			[]string{"UX-01 +flag extra"}},
		{"flag added in a driver, docs", cmdBare, drvBase, cmdBare, drvAdded, true, nil},
		{"key removed from a driver", cmdBare, drvBase, cmdBare, drvEmpty, true,
			[]string{`UX-03 key "relay_image" was removed`}},
		{"a deeper directory is not read", cmdBare, drvBase, cmdBare, drvBase, false, nil},
		{"key moved, old name kept", cmdBare, drvBase, cmdBare, drvMovedAlias, true, nil},
		{"common key added, no docs", cmdBare, drvBase, cmdBare, drvCommon, false,
			[]string{"UX-01 +key image_min_grace"}},
		{"key moved, old name dropped", cmdBare, drvBase, cmdBare, drvMovedBare, true,
			[]string{`UX-03 key "relay_image" was removed`}},
	}
	for _, tc := range cases {
		files := map[string]string{"cmd/warden/main.go": tc.baseCmd,
			"driver/container/settings.go": tc.baseDrv}
		r := gittest.New(t, files)
		change := map[string]string{"cmd/warden/main.go": tc.headCmd,
			"driver/container/settings.go":      tc.headDrv,
			"driver/container/settings_test.go": strings.Replace(drvAdded, "extra", "in-test", 1),
			"driver/container/deep/x.go":        strings.Replace(drvAdded, "extra", "deep", 1)}
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
