package specs_test

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// driverTables are the contract's tables of version 1: the reserved flag
// names by command ("" for every command), the commands a flag may name, the
// common keys with their TOML type, and the old names by driver name, each
// mapped to its key.
type driverTables struct {
	reserved     map[string][]string
	flagCommands []string
	commonKeys   map[string]string
	oldNames     map[string]map[string]string
}

// driverTableCopy is one party's copy of the tables: the Go package that
// holds it and the names of its variables, "" where it holds no such table.
// The three are kept apart on purpose (driver-contract design,
// Consolidation), so they are read from source rather than imported: the
// suite may import nothing under driver/ or cmd/warden, and cmd/warden is a
// main package.
type driverTableCopy struct {
	dir                                          string
	reserved, flagCommands, commonKeys, oldNames string
}

var driverTableCopies = []driverTableCopy{
	{"cmd/warden", "reservedFlags", "", "commonKeys", "oldKeyNames"},
	{"driver/drivercmd", "ReservedFlags", "flagCommands", "commonKeys", "oldNames"},
	{"conformance/driver", "reservedFlags", "flagCommands", "commonKeys", "oldNames"},
}

// specDriverTables reads the tables from v1.md (Reserved flag names, The
// common keys of version 1, Old names) and the flag commands from the
// schema's describe-flag.
func specDriverTables(fsys fs.FS) (*driverTables, error) {
	doc, err := fs.ReadFile(fsys, driverContractDoc)
	if err != nil {
		return nil, err
	}
	t := &driverTables{reserved: map[string][]string{}, commonKeys: map[string]string{},
		oldNames: map[string]map[string]string{}}
	code := regexp.MustCompile("`([^`]+)`")
	words := func(cell string) []string {
		var out []string
		for _, m := range code.FindAllStringSubmatch(cell, -1) {
			out = append(out, m[1])
		}
		return out
	}
	rows, err := mdTable(string(doc), "### Reserved flag names")
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		cmd := ""
		if r[0] != "Every command" {
			cmd = words(r[0])[0]
		}
		t.reserved[cmd] = words(r[1])
	}
	if rows, err = mdTable(string(doc), "### The common keys of version 1"); err != nil {
		return nil, err
	}
	for _, r := range rows {
		// The table states the type in words; every key of version 1 is "A
		// string holding an age".
		typ := "unstated: " + r[1]
		if strings.HasPrefix(r[1], "A string") {
			typ = "string"
		}
		t.commonKeys[words(r[0])[0]] = typ
	}
	if rows, err = mdTable(string(doc), "### Old names"); err != nil {
		return nil, err
	}
	for _, r := range rows {
		name := words(r[0])[0]
		if t.oldNames[name] == nil {
			t.oldNames[name] = map[string]string{}
		}
		t.oldNames[name][words(r[1])[0]] = words(r[2])[0]
	}
	schema, err := fs.ReadFile(fsys, driverContractSchema)
	if err != nil {
		return nil, err
	}
	var s struct {
		Defs struct {
			Flag struct {
				Properties struct {
					Commands struct {
						Items struct {
							Enum []string `json:"enum"`
						} `json:"items"`
					} `json:"commands"`
				} `json:"properties"`
			} `json:"describe-flag"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil, err
	}
	t.flagCommands = s.Defs.Flag.Properties.Commands.Items.Enum
	return t, nil
}

// mdTable returns the body rows of the first table after heading, each row's
// cells trimmed.
func mdTable(doc, heading string) ([][]string, error) {
	_, after, ok := strings.Cut(doc, "\n"+heading+"\n")
	if !ok {
		return nil, fmt.Errorf("%s has no %q", driverContractDoc, heading)
	}
	var rows [][]string
	for _, line := range strings.Split(after, "\n") {
		if !strings.HasPrefix(line, "|") {
			if len(rows) > 0 {
				break
			}
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows = append(rows, cells)
	}
	if len(rows) < 3 {
		return nil, fmt.Errorf("%s: no table rows under %q", driverContractDoc, heading)
	}
	return rows[2:], nil // the header and its rule
}

// goDriverTables reads c's tables from its package's source. Its common keys
// take the type "string": each of the three copies treats a common key as a
// string holding an age (warden reads it as one, the kit requires Age, the
// suite requires "string"), so a common key of any other type in v1.md fails
// the comparison.
func goDriverTables(fsys fs.FS, c driverTableCopy) (*driverTables, error) {
	vals, err := goStringValues(fsys, c.dir)
	if err != nil {
		return nil, err
	}
	get := func(name string) (any, error) {
		if name == "" {
			return nil, nil
		}
		v, ok := vals[name]
		if !ok {
			return nil, fmt.Errorf("%s has no table %s", c.dir, name)
		}
		return v, nil
	}
	t := &driverTables{}
	v, err := get(c.reserved)
	if err != nil {
		return nil, err
	}
	reserved, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s.%s is not a map", c.dir, c.reserved)
	}
	t.reserved = map[string][]string{}
	for cmd, names := range reserved {
		if cmd == "*" { // the kit's key for every command
			cmd = ""
		}
		t.reserved[cmd] = strs(names)
	}
	if v, err = get(c.flagCommands); err != nil {
		return nil, err
	}
	t.flagCommands = strs(v)
	if v, err = get(c.commonKeys); err != nil {
		return nil, err
	}
	t.commonKeys = map[string]string{}
	for _, k := range strs(v) {
		t.commonKeys[k] = "string"
	}
	if v, err = get(c.oldNames); err != nil {
		return nil, err
	}
	byName, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s.%s is not a map", c.dir, c.oldNames)
	}
	t.oldNames = map[string]map[string]string{}
	for name, m := range byName {
		t.oldNames[name] = map[string]string{}
		olds, _ := m.(map[string]any)
		for old, key := range olds {
			t.oldNames[name][old], _ = key.(string)
		}
	}
	return t, nil
}

// strs is v's strings, or nil when v is not a list of strings.
func strs(v any) []string {
	l, _ := v.([]any)
	var out []string
	for _, e := range l {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// goStringValues evaluates the package-level constants and variables of the
// non-test Go files in dir whose values are built only of string literals,
// constants, slices and maps: a string, []any or map[string]any.
func goStringValues(fsys fs.FS, dir string) (map[string]any, error) {
	files, err := fs.Glob(fsys, path.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	var specs []*ast.ValueSpec
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		af, err := parser.ParseFile(token.NewFileSet(), f, src,
			parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, d := range af.Decls {
			if g, ok := d.(*ast.GenDecl); ok {
				for _, s := range g.Specs {
					if vs, ok := s.(*ast.ValueSpec); ok {
						specs = append(specs, vs)
					}
				}
			}
		}
	}
	vals := goValues{}
	// Two passes, so that a table naming a constant declared after it resolves.
	for range 2 {
		for _, vs := range specs {
			for i, n := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				if v, ok := vals.eval(vs.Values[i]); ok {
					vals[n.Name] = v
				}
			}
		}
	}
	return vals, nil
}

// goValues are the values of a package's names evaluated so far.
type goValues map[string]any

// eval is e's value, and false when e is not built only of string literals,
// names in vals, and slice and map literals.
func (vals goValues) eval(e ast.Expr) (any, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return nil, false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.Ident:
		v, ok := vals[e.Name].(string)
		return v, ok
	case *ast.CompositeLit:
		return vals.evalComposite(e)
	}
	return nil, false
}

func (vals goValues) evalComposite(e *ast.CompositeLit) (any, bool) {
	// An element type elided in a literal leaves e.Type nil, so a map is told
	// by its keyed elements.
	_, isMap := e.Type.(*ast.MapType)
	if len(e.Elts) > 0 {
		_, isMap = e.Elts[0].(*ast.KeyValueExpr)
	}
	if !isMap {
		var l []any
		for _, el := range e.Elts {
			v, ok := vals.eval(el)
			if !ok {
				return nil, false
			}
			l = append(l, v)
		}
		return l, true
	}
	m := map[string]any{}
	for _, el := range e.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			return nil, false
		}
		k, ok1 := vals.eval(kv.Key)
		v, ok2 := vals.eval(kv.Value)
		ks, ok3 := k.(string)
		if !ok1 || !ok2 || !ok3 {
			return nil, false
		}
		m[ks] = v
	}
	return m, true
}

// checkDriverTables holds each Go copy of the contract's tables to v1.md and
// the schema.
func checkDriverTables(fsys fs.FS) error {
	want, err := specDriverTables(fsys)
	if err != nil {
		return err
	}
	sorted := func(l []string) []string { return slices.Sorted(slices.Values(l)) }
	norm := func(m map[string][]string) map[string][]string {
		out := map[string][]string{}
		for k, v := range m {
			out[k] = sorted(v)
		}
		return out
	}
	var errs []string
	for _, c := range driverTableCopies {
		got, err := goDriverTables(fsys, c)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(norm(got.reserved), norm(want.reserved)) {
			errs = append(errs, fmt.Sprintf("%s.%s = %v, v1.md's Reserved flag "+
				"names = %v", c.dir, c.reserved, got.reserved, want.reserved))
		}
		if c.flagCommands != "" &&
			!slices.Equal(sorted(got.flagCommands), sorted(want.flagCommands)) {
			errs = append(errs, fmt.Sprintf("%s.%s = %v, the schema's describe-flag "+
				"commands = %v", c.dir, c.flagCommands, got.flagCommands,
				want.flagCommands))
		}
		if !reflect.DeepEqual(got.commonKeys, want.commonKeys) {
			errs = append(errs, fmt.Sprintf("%s.%s = %v, v1.md's common keys = %v",
				c.dir, c.commonKeys, got.commonKeys, want.commonKeys))
		}
		if !reflect.DeepEqual(got.oldNames, want.oldNames) {
			errs = append(errs, fmt.Sprintf("%s.%s = %v, v1.md's Old names = %v",
				c.dir, c.oldNames, got.oldNames, want.oldNames))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return nil
}

// The three copies of the driver contract's tables, in warden, the kit and
// the suite, each equal v1.md's and the schema's.
func TestDriverContractTables(t *testing.T) {
	if err := checkDriverTables(repo()); err != nil {
		t.Error(err)
	}
}

// A copy that differs from v1.md in any of its tables fails the check.
func TestDriverContractTablesChecks(t *testing.T) {
	files := []string{driverContractDoc, driverContractSchema}
	for _, c := range driverTableCopies {
		m, err := fs.Glob(repo(), path.Join(c.dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	base := fstest.MapFS{}
	for _, f := range files {
		data, err := fs.ReadFile(repo(), f)
		if err != nil {
			t.Fatal(err)
		}
		base[f] = &fstest.MapFile{Data: data}
	}
	if err := checkDriverTables(base); err != nil {
		t.Fatalf("unedited: %v", err)
	}
	for _, tc := range []struct{ name, file, old, new string }{
		{"a reserved flag", "cmd/warden/drivercall.go", `"build-link"},`, `"link"},`},
		{"a flag command", "driver/drivercmd/drivercmd.go", `CmdImagePrepare, CmdClean}`,
			`CmdImagePrepare}`},
		{"a common key", "conformance/driver/contract.go",
			`{"image_refresh_after", "image_min_grace"}`, `{"image_refresh_after"}`},
		{"an old name", "driver/drivercmd/settings.go", `"runtime.cli":`,
			`"runtime.cmd":`},
		{"a common key's type", driverContractDoc,
			"| `image_min_grace` | A string", "| `image_min_grace` | An integer"},
	} {
		m := fstest.MapFS{}
		for k, v := range base {
			m[k] = v
		}
		src := string(base[tc.file].Data)
		if !strings.Contains(src, tc.old) {
			t.Fatalf("%s: %s has no %q", tc.name, tc.file, tc.old)
		}
		m[tc.file] = &fstest.MapFile{Data: []byte(strings.Replace(src, tc.old, tc.new, 1))}
		if err := checkDriverTables(m); err == nil {
			t.Errorf("%s: want a failure, got none", tc.name)
		}
	}
}
