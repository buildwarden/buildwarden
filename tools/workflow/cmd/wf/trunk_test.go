package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// programHere is config.yaml with o/here's program trunk recorded.
const programHere = "repository: o/here\nrepositories:\n" +
	"  - {name: o/here, trunk: main, program_trunk: program/componentization}\n"

// TestTrunkCommands: wf trunk prints the branch an item works against, and
// wf main -trunk records the program trunk's reading in program.yaml
// (FLOW-10, FLOW-20).
func TestTrunkCommands(t *testing.T) {
	dir := stateDir(t, programHere)
	queued(t, dir, "p", "1", "-program")
	queued(t, dir, "f", "2")
	if got := must(t, dir, "trunk", "-feature", "p"); got != "program/componentization\n" {
		t.Fatalf("wf trunk -feature p = %q", got)
	}
	if got := must(t, dir, "trunk", "-feature", "f"); got != "main\n" {
		t.Fatalf("wf trunk -feature f = %q", got)
	}
	must(t, dir, "main", "-trunk", "program/componentization", "-sha", "abc1234",
		"-ci", "red")
	data, err := os.ReadFile(filepath.Join(dir, "program.yaml"))
	if err != nil || !strings.Contains(string(data), "ci: red") {
		t.Fatalf("program.yaml = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.yaml")); err == nil {
		t.Fatal("wf main -trunk program/componentization wrote main.yaml")
	}
	refused(t, dir, "neither the trunk nor the program trunk", "main", "-trunk", "dev",
		"-sha", "abc1234", "-ci", "red")

	dir = stateDir(t, here)
	queued(t, dir, "p", "1", "-program")
	if got := must(t, dir, "trunk", "-feature", "p"); got !=
		"main\t(o/here has no program trunk)\n" {
		t.Fatalf("wf trunk with no program trunk = %q", got)
	}
}
