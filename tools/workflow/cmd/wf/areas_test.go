package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// smallMap is a map of the code repository codeRepo builds: docs, and x.txt
// and y.txt each in an area of its own.
func smallMap(docsMode, docsPaths, yPaths string) string {
	return "version: 1\nrepository: o/here\ncatch_all: core\nareas:\n" +
		"  core: {mode: exclusive}\n" +
		"  docs: {mode: " + docsMode + ", paths: [" + docsPaths + "]}\n" +
		"  x: {mode: exclusive, paths: [x.txt]}\n" +
		"  y: {mode: exclusive, paths: [" + yPaths + "]}\n"
}

// declaredIn puts the item in progress with the areas, its branch pushed.
func declaredIn(t *testing.T, dir, name, prio string, areas ...string) {
	t.Helper()
	queued(t, dir, name, prio, "-areas", strings.Join(areas, ","))
	edit(t, dir, name, func(f *queue.Feature) { f.Status = "in_progress" })
}

// TestAreasValidate: wf validate -areas checks a candidate map and prints
// what it does to the open items: each relation that changes, any new limit
// finding, and, with -repo, each declared item in progress whose branch has
// a path the candidate puts outside its areas (FLOW-05).
func TestAreasValidate(t *testing.T) {
	plain := smallMap("ordered", "base.txt, shared.txt", "y.txt")
	work := codeRepo(t, "x", "y")
	dir := stateDir(t, here)
	write(t, dir, "areas.yaml", plain)
	declaredIn(t, dir, "x", "1", "x", "docs")
	declaredIn(t, dir, "y", "2", "y", "docs")
	same := filepath.Join(t.TempDir(), "same.yaml")
	write(t, filepath.Dir(same), "same.yaml", plain)
	out := must(t, dir, "validate", "-areas", same, "-repo", work)
	if !strings.Contains(out, "relations: no change") ||
		!strings.Contains(out, "in-flight branches: every declared item stays inside") {
		t.Fatalf("an unchanged map printed:\n%s", out)
	}

	moved := filepath.Join(t.TempDir(), "moved.yaml")
	write(t, filepath.Dir(moved), "moved.yaml",
		smallMap("exclusive", "base.txt", "y.txt, shared.txt"))
	out, err := wf(t, dir, "validate", "-areas", moved, "-repo", work)
	if err == nil {
		t.Fatalf("a candidate that puts x outside its areas passed:\n%s", out)
	}
	for _, w := range []string{
		"x, y: disjoint -> colliding (area docs)",
		"x: shared.txt would be in y, outside its areas [x docs]",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("wf validate -areas lacks %q:\n%s", w, out)
		}
	}
	if strings.Contains(out, "y: ") && strings.Contains(out, "y: shared.txt") {
		t.Errorf("y, whose paths stay inside, is named:\n%s", out)
	}
	out, _ = wf(t, dir, "validate", "-areas", moved)
	if !strings.Contains(out, "in-flight branches: not checked") {
		t.Errorf("without -repo the line does not say the branches were not checked:\n%s",
			out)
	}

	other := filepath.Join(t.TempDir(), "other.yaml")
	write(t, filepath.Dir(other), "other.yaml",
		strings.Replace(plain, "o/here", "o/else", 1))
	refused(t, dir, "problems", "validate", "-areas", other)
	// The map in the state is not changed by a candidate.
	if m := load(t, dir).Areas; m.Areas["docs"].Mode != "ordered" {
		t.Fatal("wf validate -areas changed areas.yaml")
	}
}

// TestAreasCheck: wf areas -check compares the map with a tree and, with
// -edges, with the dependencies; it records the result in main.yaml for
// origin/main only, and exits 0 whatever it finds (FLOW-05, PROFILE-01).
func TestAreasCheck(t *testing.T) {
	plain := smallMap("ordered", "base.txt, shared.txt", "y.txt")
	work := codeRepo(t, "x", "y")
	moveMain(t, work, "stray", "stray.txt")
	dir := stateDir(t, here)
	write(t, dir, "areas.yaml", plain)
	declaredIn(t, dir, "y", "1", "y")
	out := must(t, dir, "areas", "-check", "-repo", work)
	for _, w := range []string{
		"catch-all: stray.txt is in no area's paths (it falls to core)",
		"pattern: x.txt of area x matches no file",
		"pattern: y.txt of area y matches no file: expected until y merges",
		"reaches: not compared",
		"2 differences from origin/main at ",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("wf areas -check lacks %q:\n%s", w, out)
		}
	}
	m := load(t, dir).Main
	if m == nil || m.Areas == nil || m.Areas.Differences != 2 ||
		m.Areas.SHA != gitT(t, work, "rev-parse", "origin/main") {
		t.Fatalf("main.yaml records %+v", m)
	}
	if md := queueMD(t, dir); !strings.Contains(md, "The area map differs from `main`") {
		t.Errorf("QUEUE.md does not open Needs Jeff with the difference:\n%s", md)
	}

	edges := filepath.Join(t.TempDir(), "edges")
	if err := os.WriteFile(edges, []byte("x.txt y.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out = must(t, dir, "areas", "-check", "-repo", work, "-rev", "origin/feat/x",
		"-edges", edges)
	if strings.Contains(out, "reaches: not compared") {
		t.Errorf("with -edges reaches was not compared:\n%s", out)
	}
	if got := load(t, dir).Main.Areas.SHA; got != m.Areas.SHA {
		t.Fatalf("a check of another rev was recorded: %s", got)
	}

	out = must(t, dir, "areas", "-list")
	if !strings.Contains(out, "docs\tordered\tpaths: base.txt, shared.txt") ||
		!strings.Contains(out, "core\texclusive\tcatch-all") {
		t.Errorf("wf areas -list printed:\n%s", out)
	}
}
