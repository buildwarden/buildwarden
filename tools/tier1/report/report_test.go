package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func sample() *Report {
	r := &Report{Base: "0123456789abcdef", Head: "fedcba9876543210"}
	r.Add(
		Finding{Rule: "RELAY-07", Severity: Must, File: "relay/x.go", Line: 9,
			Check: "relay-diskless", Message: "os.Create in relay/"},
		Finding{Rule: "EXEC-05", Severity: Should, File: "cmd/warden-io/a.go", Line: 3,
			Check: "large-literal", Message: "string literal of 5000 bytes"},
		Finding{Rule: "GLOBAL-04", Severity: Hint, File: "relay",
			Check:   "tests-with-change",
			Message: "non-test code changed, no test changed"},
	)
	r.Suppressed = []Suppression{{Rule: "UX-01", Path: "cmd/warden/*", Reason: "docs follow",
		Commit: "aaaaaaaaaaaaaaaa"}}
	r.Finish()
	return r
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs; rerun with UPDATE_GOLDEN=1\ngot:\n%s\nwant:\n%s",
			name, got, want)
	}
}

func TestGolden(t *testing.T) {
	r := sample()
	var text, js bytes.Buffer
	r.Text(&text)
	if err := r.JSON(&js); err != nil {
		t.Fatal(err)
	}
	golden(t, "report.txt", text.Bytes())
	golden(t, "report.json", js.Bytes())
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		sev  []Severity
		want int
	}{
		{"empty", nil, ExitClean},
		{"should only", []Severity{Should}, ExitClean},
		{"hint only", []Severity{Hint}, ExitClean},
		{"should and hint", []Severity{Should, Hint}, ExitClean},
		{"must", []Severity{Should, Must}, ExitFail},
		{"must-review", []Severity{MustReview}, ExitFail},
	}
	for _, c := range cases {
		r := &Report{}
		for _, s := range c.sev {
			r.Add(Finding{Rule: "X-01", Severity: s})
		}
		if got := r.ExitCode(); got != c.want {
			t.Errorf("%s: exit %d, want %d", c.name, got, c.want)
		}
	}
}

// TestSchemaDeclaresEveryKey pins report.schema.json to what JSON emits: every
// emitted key is declared, and every declared required key is emitted.
func TestSchemaDeclaresEveryKey(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "report.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
		Defs       map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	var sub struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Items      struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"items"`
	}
	var buf bytes.Buffer
	if err := sample().JSON(&buf); err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	declared(t, "top level", keys(out), schema.Properties)
	finding := schema.Defs["finding"].Properties
	for _, list := range []string{"findings", "hints"} {
		var items []map[string]json.RawMessage
		_ = json.Unmarshal(out[list], &items)
		for _, it := range items {
			declared(t, list, keys(it), finding)
		}
	}
	_ = json.Unmarshal(schema.Properties["suppressed"], &sub)
	var supp []map[string]json.RawMessage
	_ = json.Unmarshal(out["suppressed"], &supp)
	for _, it := range supp {
		declared(t, "suppressed", keys(it), sub.Items.Properties)
	}
	_ = json.Unmarshal(schema.Properties["counts"], &sub)
	var counts map[string]json.RawMessage
	_ = json.Unmarshal(out["counts"], &counts)
	declared(t, "counts", keys(counts), sub.Properties)
	for _, k := range schema.Required {
		if _, ok := out[k]; !ok {
			t.Errorf("required key %q not emitted", k)
		}
	}
}

func keys(m map[string]json.RawMessage) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func declared(t *testing.T, where string, got []string, props map[string]json.RawMessage) {
	t.Helper()
	if len(got) == 0 {
		t.Errorf("%s: nothing emitted", where)
	}
	for _, k := range got {
		if _, ok := props[k]; !ok {
			t.Errorf("%s: key %q not declared in report.schema.json", where, k)
		}
	}
}
