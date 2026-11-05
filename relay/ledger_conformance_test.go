package relay

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/buildwarden/buildwarden/ledger"
)

// specSchema is the part of a specs/*/v*.schema.json file the conformance
// check needs: its top-level property names and whether others are forbidden.
type specSchema struct {
	file       string
	properties map[string]bool
	closed     bool
}

// loadSpecSchemas maps each published schema's $id to its top-level shape.
// io/fs paths use forward slashes, so the glob is the same on every platform.
func loadSpecSchemas(t *testing.T) map[string]specSchema {
	t.Helper()
	fsys := os.DirFS("..")
	files, err := fs.Glob(fsys, "specs/*/v*.schema.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob specs/*/v*.schema.json = %v, %v", files, err)
	}
	schemas := map[string]specSchema{}
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var s struct {
			ID         string                     `json:"$id"`
			Properties map[string]json.RawMessage `json:"properties"`
			// Raw, because an absent keyword (any key allowed) differs from
			// false, and a schema-valued keyword is not a bool at all.
			Additional json.RawMessage `json:"additionalProperties"`
		}
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		props := map[string]bool{}
		for k := range s.Properties {
			props[k] = true
		}
		schemas[s.ID] = specSchema{
			file:       f,
			properties: props,
			closed:     bytes.Equal(bytes.TrimSpace(s.Additional), []byte("false")),
		}
	}
	return schemas
}

// TestLedgerMetadataConformsToSpecs checks every record of the end-to-end
// scenario's ledger against the schema its header names (RELAY-05): each
// schema index resolves to a published schema, and a schema that forbids
// additional properties sees no top-level key outside its properties. The
// check is shallow on purpose: it needs no JSON Schema validator and still
// catches a producer writing a field the schema does not define.
func TestLedgerMetadataConformsToSpecs(t *testing.T) {
	out := runEndToEndScenario(t)
	data, err := os.ReadFile(filepath.Join(out, "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := ledger.Verify(data)
	if err != nil {
		t.Fatal(err)
	}
	schemas := loadSpecSchemas(t)
	metas := make([]map[string]any, len(res.Records))
	for i, rec := range res.Records {
		metas[i] = checkRecordMetadata(t, i, rec, res.Header.Meta.Schemas, schemas)
	}
	for _, url := range []string{
		"http://artifacts/build-script",
		"http://artifacts/bootstrap",
		"http://cwd/src/main.c",
	} {
		checkServedClose(t, url, res.Records, metas)
	}
}

// checkRecordMetadata resolves one record's schema index through the header's
// schema list to a published schema, reports any top-level key a closed schema
// does not define, and returns the decoded metadata (nil when there is none or
// it does not resolve).
func checkRecordMetadata(
	t *testing.T, i int, rec ledger.Record, ids []string, schemas map[string]specSchema,
) map[string]any {
	t.Helper()
	if rec.SchemaIndex == 0xFF {
		return nil
	}
	if int(rec.SchemaIndex) >= len(ids) {
		t.Errorf("record %d (%s): schema index %d outside the header's %d schemas",
			i, rec.TypeName(), rec.SchemaIndex, len(ids))
		return nil
	}
	id := ids[rec.SchemaIndex]
	s, ok := schemas[id]
	if !ok {
		t.Errorf("record %d (%s): $id %s is not a specs/*/v*.schema.json file",
			i, rec.TypeName(), id)
		return nil
	}
	var m map[string]any
	if err := cbor.Unmarshal(rec.Metadata, &m); err != nil {
		t.Errorf("record %d (%s): metadata under %s is not a CBOR map: %v",
			i, rec.TypeName(), id, err)
		return nil
	}
	if s.closed {
		for k := range m {
			if !s.properties[k] {
				t.Errorf("record %d (%s): key %q is not a property of %s (%s)",
					i, rec.TypeName(), k, id, s.file)
			}
		}
	}
	return m
}

// checkServedClose asserts that the relay-served input fetched from url closes
// as an http-body response body with exactly {"status": 200}.
func checkServedClose(t *testing.T, url string, recs []ledger.Record, metas []map[string]any) {
	t.Helper()
	open := -1
	for i, rec := range recs {
		if rec.Type == ledger.RecordOpen && metas[i]["url"] == url {
			open = i
			break
		}
	}
	if open < 0 {
		t.Errorf("no OPEN record for %s", url)
		return
	}
	closed := false
	for i, rec := range recs {
		if rec.Type != ledger.RecordClose ||
			!bytes.Equal(rec.OpenSig, recs[open].Signature) {
			continue
		}
		closed = true
		if rec.SchemaIndex != schemaHTTPBody {
			t.Errorf("%s CLOSE schema index = %d, want %d (http-body)",
				url, rec.SchemaIndex, schemaHTTPBody)
		}
		want := map[string]any{"status": uint64(http.StatusOK)}
		if !reflect.DeepEqual(metas[i], want) {
			t.Errorf("%s CLOSE metadata = %v, want %v", url, metas[i], want)
		}
	}
	if !closed {
		t.Errorf("no CLOSE record for %s", url)
	}
}
