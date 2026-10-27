package script

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A file copied into a directory dest lands by its base name, as docker
// places it, not under its context path.
func TestTranslateCopyFileIntoDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "Dockerfile")
	df := "FROM alpine:3.20\nCOPY sub/deep/d.txt a.txt /opt/\n"
	if err := os.WriteFile(p, []byte(df), 0o644); err != nil {
		t.Fatal(err)
	}
	const nl = "\n"
	r, err := Translate(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-o /opt/d.txt" + nl, "-o /opt/a.txt" + nl} {
		if !strings.Contains(r.Script, want) {
			t.Errorf("script lacks %s:\n%s", want, r.Script)
		}
	}
}
