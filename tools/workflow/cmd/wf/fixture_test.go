package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture is a copy of the workflow branch's state at c0b3295, from before
// work and repositories existed, with what the wf of main at e9315c2 printed
// on it: wf next, wf status -md and wf validate.
const fixture = "testdata/c0b3295"

// copyFixture extracts the fixture's state, kept as state.tar.gz so that
// the repository's text scans do not read the queue's history as source,
// into a new directory and appends the repositories entries to its
// config.yaml.
func copyFixture(t *testing.T, entries string) string {
	t.Helper()
	dir := t.TempDir()
	f, err := os.Open(filepath.Join(fixture, "state.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(z)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(dir, filepath.FromSlash(h.Name))
		if h.Typeflag == tar.TypeDir {
			_ = os.MkdirAll(name, 0o755)
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		_ = os.MkdirAll(filepath.Dir(name), 0o755)
		if err := os.WriteFile(name, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, append(data, []byte(entries)...), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestStateBeforeRepositories: the state as it stood before this change,
// with the repositories entries as they stand after task 14b, gives the
// same wf next, wf status -md and wf validate as the wf before it, the
// in-progress relay-collector-failure-fails-open, which names
// buildwarden/buildwarden-specs, among the items; with only task 14a's
// entries, this repository reads as not adopted (Validation plan test 10,
// a10, Decision 3).
func TestStateBeforeRepositories(t *testing.T) {
	const after14b = "repositories:\n" +
		"  - name: buildwarden/buildwarden\n    trunk: main\n" +
		"    profile: docs/development/loop-profile.yaml\n" +
		"  - name: buildwarden/buildwarden-specs\n    trunk: main\n"
	at := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	dir := copyFixture(t, after14b)
	if _, err := os.Stat(filepath.Join(dir, "queue",
		"relay-collector-failure-fails-open.yaml")); err != nil {
		t.Fatalf("the fixture lacks the item that names the specs repository: %v", err)
	}
	for _, c := range []struct {
		file string
		args []string
	}{
		{"next.txt", []string{"next", "-worker", "cloud/fixture"}},
		{"status.md", []string{"status", "-md"}},
		{"validate.txt", []string{"validate"}},
	} {
		want, err := os.ReadFile(filepath.Join(fixture, c.file))
		if err != nil {
			t.Fatal(err)
		}
		got, err := wfAt(t, dir, at, c.args[0], c.args[1:]...)
		if err != nil || got != string(want) {
			t.Errorf("wf %s: err %v\n got %q\nwant %q", strings.Join(c.args, " "), err,
				got, want)
		}
	}

	const after14a = "repositories:\n" +
		"  - name: buildwarden/buildwarden\n    trunk: main\n" +
		"  - name: buildwarden/buildwarden-specs\n    trunk: main\n"
	dir = copyFixture(t, after14a)
	out, err := wfAt(t, dir, at, "validate")
	if err == nil || strings.Contains(out, "has no entry") ||
		!strings.Contains(out, `a tracked item's round is tracked, not "feature"`) {
		t.Errorf("with task 14a's entries only: err %v\n%s", err, out)
	}
}
