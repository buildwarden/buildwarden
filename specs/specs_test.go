// Package specs_test ties the ledger producer, the specification documents
// under specs/ and the book's mirror of them together (RELAY-05). It holds no
// code of its own: specs/ is a documentation tree, and this test only reads
// it, through the exported relay and ledger APIs.
package specs_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
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

func TestRepoSpecs(t *testing.T) {
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
