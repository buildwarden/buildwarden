package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

var now = time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

// stateDir is a checkout of the workflow branch with the config and no item.
func stateDir(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "queue"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("run_budget: {steps: 8}\n"+config), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// wf runs one wf command on the state checkout and returns what it printed
// and its error.
func wf(t *testing.T, dir, cmd string, args ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	runErr := run(cmd, append([]string{"-state", dir}, args...), now)
	os.Stdout = stdout
	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), runErr
}

// must is wf for a command that has to succeed.
func must(t *testing.T, dir, cmd string, args ...string) string {
	t.Helper()
	out, err := wf(t, dir, cmd, args...)
	if err != nil {
		t.Fatalf("wf %s %v: %v", cmd, args, err)
	}
	return out
}

// refused asserts that the command fails with the message.
func refused(t *testing.T, dir, want, cmd string, args ...string) {
	t.Helper()
	_, err := wf(t, dir, cmd, args...)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("wf %s %v: %v, want %q", cmd, args, err, want)
	}
}

// queued files a feature with wf new and returns its name.
func queued(t *testing.T, dir, name string, priority string, more ...string) string {
	t.Helper()
	must(t, dir, "new", append([]string{"-worker", "human/jeff", "-feature", name,
		"-title", "The item " + name, "-priority", priority, "-estimate-steps", "9"},
		more...)...)
	return name
}

// load reads the state checkout.
func load(t *testing.T, dir string) *queue.State {
	t.Helper()
	s, err := queue.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// item is the named item as the state checkout has it.
func item(t *testing.T, dir, name string) *queue.Feature {
	t.Helper()
	f, err := load(t, dir).Find(name)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestClaimIsGuarded: wf claim applies FLOW-05's limits to every worker, as
// wf next does, and a step the worker already holds stays claimable (FLOW-04,
// FLOW-05; case 5 of the loop-parallel design).
func TestClaimIsGuarded(t *testing.T) {
	dir := stateDir(t, "limits: {in_progress: 1}\n")
	a, b := queued(t, dir, "a", "1"), queued(t, dir, "b", "2")
	claim := func(name, worker string) []string {
		return []string{"-worker", worker, "-feature", name, "-step", "f0-branch-setup"}
	}
	must(t, dir, "claim", claim(a, "cloud/run-1")...)
	const full = "f0-branch-setup may not be claimed: the in-progress limit (limit 1): " +
		"it collides with a (FLOW-05)"
	refused(t, dir, full, "claim", claim(b, "cloud/run-2")...)
	const line = "blocked\tb\tthe in-progress limit (limit 1): it collides with a " +
		"(FLOW-05)\nnone\n"
	if out := must(t, dir, "next", "-worker", "cloud/run-2", "-feature", b); out != line {
		t.Fatalf("wf next -feature b printed %q", out)
	}
	// The holder claims its own step again: a holder is never refused.
	must(t, dir, "claim", claim(a, "cloud/run-1")...)
	if got := item(t, dir, b).Status; got != "queued" {
		t.Fatalf("the refused claim left b %s", got)
	}
	must(t, dir, "validate")
}
