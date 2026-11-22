// Package specs_test ties the ledger producer, the specification documents
// under specs/ and the book's mirror of them together (RELAY-05), and the
// book's seams page to the pins of the seam specifications under
// tools/conformance/ (TEST-05). It holds no code of its own: specs/ is a
// documentation tree, and this test only reads it, through the exported relay
// and ledger APIs.
package specs_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/buildwarden/buildwarden/ledger"
	"github.com/buildwarden/buildwarden/relay"
)

const draft202012 = "https://json-schema.org/draft/2020-12/schema"

// repo is the repository root as an fs.FS. io/fs paths always use forward
// slashes, so every check below is the same on Windows.
func repo() fs.FS { return os.DirFS("..") }

// schemaIDs maps each specs/*/v*.schema.json file's $id to the files that
// declare it, after checking that each parses and declares the 2020-12 draft.
func schemaIDs(fsys fs.FS) (map[string][]string, error) {
	files, err := fs.Glob(fsys, "specs/*/v*.schema.json")
	if err != nil {
		return nil, fmt.Errorf("glob schemas: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no specs/*/v*.schema.json found")
	}
	ids := map[string][]string{}
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var s struct {
			Schema string `json:"$schema"`
			ID     string `json:"$id"`
		}
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if s.Schema != draft202012 {
			return nil, fmt.Errorf("%s: $schema = %q, want %q",
				f, s.Schema, draft202012)
		}
		if s.ID == "" {
			return nil, fmt.Errorf("%s: no $id", f)
		}
		ids[s.ID] = append(ids[s.ID], f)
	}
	return ids, nil
}

// checkSchemas fails when two schema files share an $id.
func checkSchemas(fsys fs.FS) error {
	ids, err := schemaIDs(fsys)
	if err != nil {
		return err
	}
	for id, files := range ids {
		if len(files) > 1 {
			return fmt.Errorf("$id %s declared by %d files: %s",
				id, len(files), strings.Join(files, ", "))
		}
	}
	return nil
}

// checkHeaderURIs fails unless each URI is the $id of exactly one schema file.
func checkHeaderURIs(fsys fs.FS, uris []string) error {
	ids, err := schemaIDs(fsys)
	if err != nil {
		return err
	}
	for _, u := range uris {
		if n := len(ids[u]); n != 1 {
			return fmt.Errorf("header schema %s is the $id of %d files, want 1", u, n)
		}
	}
	return nil
}

// checkBookMirror fails unless every specs/ document has a mirror page under
// docs/book/src/specs/ that includes it, listed in SUMMARY.md.
func checkBookMirror(fsys fs.FS) error {
	docs, err := fs.Glob(fsys, "specs/*/v*.md")
	if err != nil {
		return fmt.Errorf("glob documents: %w", err)
	}
	docs = append(docs, "specs/README.md")
	sort.Strings(docs)
	summary, err := fs.ReadFile(fsys, "docs/book/src/SUMMARY.md")
	if err != nil {
		return fmt.Errorf("read SUMMARY.md: %w", err)
	}
	for _, doc := range docs {
		mirror := path.Join("docs/book/src", doc)
		page, err := fs.ReadFile(fsys, mirror)
		if err != nil {
			return fmt.Errorf("%s has no book mirror page: %w", doc, err)
		}
		// The mirror sits under docs/book/src/, three directories below
		// the root, plus one per segment of the document's own path.
		up := strings.Repeat("../", 3+strings.Count(doc, "/"))
		want := "{{#include " + up + doc + "}}"
		if strings.TrimSpace(string(page)) != want {
			return fmt.Errorf("%s: want exactly %q", mirror, want)
		}
		entry := "(./" + strings.TrimPrefix(mirror, "docs/book/src/") + ")"
		if !bytes.Contains(summary, []byte(entry)) {
			return fmt.Errorf("SUMMARY.md does not list %s", mirror)
		}
	}
	return nil
}

// headerSchemas returns the schemas array of a ledger written by the relay
// with its defaults.
func headerSchemas(t *testing.T) []string {
	t.Helper()
	var buf bytes.Buffer
	l, err := relay.NewLedger(relay.LedgerConfig{Writer: &buf})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	l.Finish()
	h, _, err := ledger.ReadHeader(buf.Bytes())
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}
	if len(h.Meta.Schemas) == 0 {
		t.Fatal("relay header lists no schemas")
	}
	return h.Meta.Schemas
}

// draftMark opens a working draft's status block (FLOW-20).
const draftMark = "> **Working draft, unreleased (FLOW-20).**"

// draftListed is how the index names a working draft.
const draftListed = "(working draft, unreleased)"

// checkDrafts fails a specs/ document that opens with a working draft's
// status block when the index does not list it as unreleased, or when base,
// the branch a pull request targets or a push is to, is main: a draft
// reaches main only finalized, when the program lands (FLOW-20, RELAY-05).
func checkDrafts(fsys fs.FS, base string) error {
	docs, err := fs.Glob(fsys, "specs/*/v*.md")
	if err != nil {
		return fmt.Errorf("glob documents: %w", err)
	}
	index, err := fs.ReadFile(fsys, "specs/README.md")
	if err != nil {
		return fmt.Errorf("read the index: %w", err)
	}
	for _, doc := range docs {
		data, err := fs.ReadFile(fsys, doc)
		if err != nil {
			return fmt.Errorf("read %s: %w", doc, err)
		}
		if !bytes.Contains(data, []byte(draftMark)) {
			continue
		}
		if base == "main" {
			return fmt.Errorf("%s is a working draft: it reaches main only "+
				"finalized, when the program lands (FLOW-20)", doc)
		}
		link := "(" + strings.TrimPrefix(doc, "specs/") + ")"
		listed := false
		for _, line := range strings.Split(string(index), "\n") {
			if strings.Contains(line, link) && strings.Contains(line, draftListed) {
				listed = true
			}
		}
		if !listed {
			return fmt.Errorf("specs/README.md does not list %s as %s", doc,
				draftListed)
		}
	}
	return nil
}

// ciBase is the branch CI checks against: a pull request's base, or the
// branch a push is to; empty outside CI.
func ciBase() string {
	if b := os.Getenv("GITHUB_BASE_REF"); b != "" {
		return b
	}
	return os.Getenv("GITHUB_REF_NAME")
}

// TestDrafts: a working draft is listed as unreleased and kept off main
// (FLOW-20).
func TestDrafts(t *testing.T) {
	const draftDoc = "specs/build-ledger/v2.md"
	draft := file("# Ledger wire format, v2\n\n" + draftMark + " Not a released version " +
		"of the ledger format.\n")
	listed := file("# index\n\n- [Ledger wire format, v2 (working draft, unreleased)]" +
		"(build-ledger/v2.md)\n")
	for _, tc := range []struct {
		name, base string
		index      *fstest.MapFile
		ok         bool
	}{
		{"listed, to the program trunk", "program/componentization", listed, true},
		{"listed, no CI", "", listed, true},
		{"to main", "main", listed, false},
		{"not listed", "program/componentization", file("# index\n"), false},
	} {
		m := fixture()
		m[draftDoc], m["specs/README.md"] = draft, tc.index
		if err := checkDrafts(m, tc.base); (err == nil) != tc.ok {
			t.Errorf("%s: checkDrafts = %v, want ok %v", tc.name, err, tc.ok)
		}
	}
	if err := checkDrafts(fixture(), "main"); err != nil {
		t.Errorf("no draft, on main: %v", err)
	}
	t.Setenv("GITHUB_BASE_REF", "")
	t.Setenv("GITHUB_REF_NAME", "main")
	if got := ciBase(); got != "main" {
		t.Errorf("a push to main: ciBase = %q", got)
	}
	t.Setenv("GITHUB_BASE_REF", "program/componentization")
	if got := ciBase(); got != "program/componentization" {
		t.Errorf("a pull request: ciBase = %q", got)
	}
}

func TestRepoSpecs(t *testing.T) {
	if err := checkDrafts(repo(), ciBase()); err != nil {
		t.Error(err)
	}
	if err := checkHeaderURIs(repo(), headerSchemas(t)); err != nil {
		t.Error(err)
	}
	if err := checkSchemas(repo()); err != nil {
		t.Error(err)
	}
	if err := checkBookMirror(repo()); err != nil {
		t.Error(err)
	}
}

// file returns an in-memory file holding text.
func file(text string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(text)} }

func schemaFile(id string) *fstest.MapFile {
	return file(`{"$schema": "` + draft202012 + `", "$id": "` + id + `"}`)
}

const (
	schemaA  = "specs/metadata-a/v1.schema.json"
	mirrorA  = "docs/book/src/specs/metadata-a/v1.md"
	summary  = "docs/book/src/SUMMARY.md"
	includeA = "{{#include ../../../../../specs/metadata-a/v1.md}}\n"

	includeIndex = "{{#include ../../../../specs/README.md}}\n"
)

// fixture is a minimal well-formed tree: one schema with its document, the
// index, their mirror pages and a SUMMARY.md listing both.
func fixture() fstest.MapFS {
	return fstest.MapFS{
		"specs/README.md":               file("# index\n"),
		"specs/metadata-a/v1.md":        file("# a\n"),
		schemaA:                         schemaFile("urn:a"),
		"docs/book/src/specs/README.md": file(includeIndex),
		mirrorA:                         file(includeA),
		summary: file("- [S](./specs/README.md)\n" +
			"  - [a](./specs/metadata-a/v1.md)\n"),
	}
}

func TestChecks(t *testing.T) {
	tests := []struct {
		name   string
		edit   func(fstest.MapFS)
		uris   []string
		wantOK bool
	}{
		{name: "clean", wantOK: true},
		{name: "header URI with no file", uris: []string{"urn:missing"}},
		{
			name: "duplicate $id",
			edit: func(m fstest.MapFS) {
				m["specs/metadata-b/v1.schema.json"] = schemaFile("urn:a")
			},
		},
		{
			name: "wrong $schema",
			edit: func(m fstest.MapFS) { m[schemaA] = file(`{"$id": "urn:a"}`) },
		},
		{
			name: "schema is not JSON",
			edit: func(m fstest.MapFS) { m[schemaA] = file(`{`) },
		},
		{
			name: "missing mirror page",
			edit: func(m fstest.MapFS) { delete(m, mirrorA) },
		},
		{
			name: "mirror includes the wrong path",
			edit: func(m fstest.MapFS) {
				m[mirrorA] = file(strings.Replace(includeA, "../", "", 1))
			},
		},
		{
			name: "mirror not in SUMMARY.md",
			edit: func(m fstest.MapFS) {
				m[summary] = file("- [S](./specs/README.md)\n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := fixture()
			if tt.edit != nil {
				tt.edit(m)
			}
			uris := append([]string{"urn:a"}, tt.uris...)
			var errs []error
			for _, err := range []error{
				checkHeaderURIs(m, uris), checkSchemas(m), checkBookMirror(m),
			} {
				if err != nil {
					errs = append(errs, err)
				}
			}
			if tt.wantOK && len(errs) > 0 {
				t.Fatalf("want clean, got %v", errs)
			}
			if !tt.wantOK && len(errs) == 0 {
				t.Fatal("want a failure, got none")
			}
		})
	}
}

// The seam specifications this repository pins, each in its own module
// tools/conformance/<directory>/go.mod (Decision 7 of
// docs/design/seam-specs.md), and the book page that links them.
const (
	specsRepo = "github.com/buildwarden/buildwarden-specs"
	seamsPage = "docs/book/src/concepts/seams.md"
)

var seamSpecs = []string{
	"bootstrap-config", "collector-inputs", "collector-outputs",
	"executor-initialize", "relay-claim", "relay-guest-api",
}

var (
	// pseudoRev captures the commit a pseudo-version names.
	pseudoRev = regexp.MustCompile(`[0-9]{14}-([0-9a-f]{12})$`)
	// specsLink captures the path after blob/ or tree/ of a link into the
	// specs repository: <ref>/<directory>/<one segment>.
	specsLink = regexp.MustCompile(
		`https://github\.com/buildwarden/buildwarden-specs/(?:blob|tree)/([^)\s]+)\)`)
	// pinCell captures a row's pin, shown as `<directory>@<version>`.
	pinCell = regexp.MustCompile("`([a-z-]+)@([^`]+)`")
)

// seamRef is the ref the seams page links a specification at, for the version
// its module pins: the tag <directory>/<version> for a release, and the
// twelve-digit commit of a pseudo-version. tools/conformance/pin-specs.sh
// writes the page with the same form.
func seamRef(dir, version string) string {
	if m := pseudoRev.FindStringSubmatch(version); m != nil {
		return m[1]
	}
	return dir + "/" + version
}

// pinnedVersion returns the version tools/conformance/<dir>/go.mod requires
// the specification's module at, in the one-line or the block form.
func pinnedVersion(fsys fs.FS, dir string) (string, error) {
	file := "tools/conformance/" + dir + "/go.mod"
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return "", fmt.Errorf("read pin module: %w", err)
	}
	mod := specsRepo + "/" + dir
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && f[0] == "require" {
			f = f[1:]
		}
		if len(f) >= 2 && f[0] == mod {
			return f[1], nil
		}
	}
	return "", fmt.Errorf("%s does not require %s", file, mod)
}

// checkSeamRefs fails unless the seams page shows each specification's pin as
// its module states it, and every link into the specs repository names a
// specification at the ref of that pin.
func checkSeamRefs(fsys fs.FS) error {
	page, err := fs.ReadFile(fsys, seamsPage)
	if err != nil {
		return fmt.Errorf("read seams page: %w", err)
	}
	pins := map[string]string{}
	for _, dir := range seamSpecs {
		if pins[dir], err = pinnedVersion(fsys, dir); err != nil {
			return err
		}
	}
	shown := map[string]bool{}
	for _, m := range pinCell.FindAllStringSubmatch(string(page), -1) {
		dir, version := m[1], m[2]
		want, ok := pins[dir]
		if !ok {
			continue
		}
		if shown[dir] {
			return fmt.Errorf("%s shows the pin of %s twice", seamsPage, dir)
		}
		shown[dir] = true
		if version != want {
			return fmt.Errorf("%s shows %s@%s; its module pins %s",
				seamsPage, dir, version, want)
		}
	}
	linked := map[string]bool{}
	for _, m := range specsLink.FindAllStringSubmatch(string(page), -1) {
		parts := strings.Split(m[1], "/")
		if len(parts) < 3 {
			return fmt.Errorf("%s: link %s names no specification", seamsPage, m[0])
		}
		dir := parts[len(parts)-2]
		ref := strings.Join(parts[:len(parts)-2], "/")
		want, ok := pins[dir]
		if !ok {
			return fmt.Errorf("%s: link %s is to %s, which is not pinned",
				seamsPage, m[0], dir)
		}
		if ref != seamRef(dir, want) {
			return fmt.Errorf("%s: link %s is at %s; %s is pinned at %s",
				seamsPage, m[0], ref, dir, seamRef(dir, want))
		}
		linked[parts[len(parts)-1]+" "+dir] = true
	}
	for _, dir := range seamSpecs {
		if !shown[dir] {
			return fmt.Errorf("%s does not show the pin of %s", seamsPage, dir)
		}
		if !linked["v1.md "+dir] {
			return fmt.Errorf("%s does not link %s/v1.md", seamsPage, dir)
		}
	}
	return nil
}

func TestSeamRefs(t *testing.T) {
	if err := checkSeamRefs(repo()); err != nil {
		t.Error(err)
	}
}

const (
	specsURL   = "https://" + specsRepo
	releasePin = "v0.1.0"
	commitPin  = "v0.0.0-20261116000050-bc03f6fdc96e"
)

// atTag and atCommit give the ref a fixture's page links a directory at.
func atTag(dir string) string { return dir + "/v0.1.0" }
func atCommit(string) string  { return "bc03f6fdc96e" }

// seamFixture is a tree whose six pin modules each pin version, in the block
// form go mod tidy writes, and whose seams page shows each pin and links each
// specification at ref(directory).
func seamFixture(version string, ref func(string) string) fstest.MapFS {
	m := fstest.MapFS{}
	var page strings.Builder
	page.WriteString("| Specification | Pinned at |\n|---|---|\n")
	for _, dir := range seamSpecs {
		m["tools/conformance/"+dir+"/go.mod"] = file("module x/" + dir +
			"\n\ngo 1.26.3\n\nrequire (\n" +
			"\t" + specsRepo + "/conformance v0.1.0 // indirect\n" +
			"\t" + specsRepo + "/" + dir + " " + version + " // indirect\n)\n")
		fmt.Fprintf(&page,
			"| [%s](%s/blob/%s/%s/v1.md) ([cases](%s/tree/%s/%s/cases)) | `%s@%s` |\n",
			dir, specsURL, ref(dir), dir, specsURL, ref(dir), dir, dir, version)
	}
	m[seamsPage] = file(page.String())
	return m
}

// oneLine is relay-claim's pin module with its requirement in the one-line
// form, at version.
func oneLine(version string) string {
	return "module x\n\nrequire " + specsRepo + "/relay-claim " + version + "\n"
}

func TestSeamRefChecks(t *testing.T) {
	rc := "tools/conformance/relay-claim/go.mod"
	tests := []struct {
		name   string
		fsys   fstest.MapFS
		edit   func(fstest.MapFS)
		wantOK bool
	}{
		{name: "release", fsys: seamFixture(releasePin, atTag), wantOK: true},
		{name: "commit", fsys: seamFixture(commitPin, atCommit), wantOK: true},
		{
			name: "one-line require",
			fsys: seamFixture(releasePin, atTag),
			edit: func(m fstest.MapFS) {
				m[rc] = file(oneLine("v0.1.0"))
			},
			wantOK: true,
		},
		{name: "commit pinned, page at a tag", fsys: seamFixture(commitPin, atTag)},
		{name: "release pinned, page at a commit",
			fsys: seamFixture(releasePin, atCommit)},
		{
			name: "module moved without the page",
			fsys: seamFixture(releasePin, atTag),
			edit: func(m fstest.MapFS) {
				m[rc] = file(oneLine("v0.2.0"))
			},
		},
		{
			name: "pin module missing",
			fsys: seamFixture(releasePin, atTag),
			edit: func(m fstest.MapFS) { delete(m, rc) },
		},
		{
			name: "pin not shown",
			fsys: seamFixture(releasePin, atTag),
			edit: func(m fstest.MapFS) {
				m[seamsPage] = file(strings.ReplaceAll(string(m[seamsPage].Data),
					"`relay-claim@v0.1.0`", "v0.1.0"))
			},
		},
		{
			name: "link to a directory that is not pinned",
			fsys: seamFixture(releasePin, atTag),
			edit: func(m fstest.MapFS) {
				m[seamsPage] = file(string(m[seamsPage].Data) +
					"[x](" + specsURL + "/blob/x/v1/build-ledger/v1.md)\n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.fsys
			if tt.edit != nil {
				tt.edit(m)
			}
			err := checkSeamRefs(m)
			if tt.wantOK && err != nil {
				t.Fatalf("want clean, got %v", err)
			}
			if !tt.wantOK && err == nil {
				t.Fatal("want a failure, got none")
			}
		})
	}
}
