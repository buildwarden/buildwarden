package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/check"
	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// dueHead is the program trunk head the fake tag-next checked.
const dueHead = "0123456789abcdef0123456789abcdef01234567"

// fakeNextTags makes tag-next name the tags at dueHead, as check.TagNext
// would on a program trunk whose checks passed, for the test's length.
func fakeNextTags(t *testing.T, tags ...string) {
	t.Helper()
	was := nextTags
	nextTags = func(check.Env, string) ([]string, string, string, error) {
		return tags, dueHead, "", nil
	}
	t.Cleanup(func() { nextTags = was })
}

// TestTagNextDue: tag-next -due appends one tag-due record per tag at the
// head it checked and prints it due; it needs -worker. tags-due lists each
// tag due until its tag record, only in a repository the machine lists, and
// record refuses a second tag-due or tag for one repository and tag
// (FLOW-17).
func TestTagNextDue(t *testing.T) {
	dir := stateDir(t, programHere)
	fakeNextTags(t, "conformance/v0.1.0-dev.1", "relay-claim/v0.2.0-dev.1")
	refused(t, dir, "needs -worker", "tag-next", "-due")
	out := must(t, dir, "tag-next", "-due", "-worker", "cloud/run")
	want := "due\to/here\tconformance/v0.1.0-dev.1\t" + dueHead + "\n" +
		"due\to/here\trelay-claim/v0.2.0-dev.1\t" + dueHead + "\n"
	if out != want {
		t.Fatalf("tag-next -due printed %q, want %q", out, want)
	}
	rs, err := queue.LoadRecords(dir)
	if err != nil || len(rs) != 2 {
		t.Fatalf("records %v, %v", rs, err)
	}
	for _, r := range rs {
		if r.Kind != "tag-due" || r.Repo != "o/here" || r.Head != dueHead ||
			r.By != "cloud/run" {
			t.Errorf("record %+v", r)
		}
	}
	if out := must(t, dir, "tag-next"); strings.Contains(out, "due") {
		t.Errorf("tag-next without -due recorded: %q", out)
	}

	tagsDueOnMachines(t, dir)
}

// dueLines runs tags-due and returns its lines with tabs as "|".
func dueLines(t *testing.T, dir string, args ...string) []string {
	t.Helper()
	return strings.Fields(strings.ReplaceAll(must(t, dir, "tags-due", args...), "\t", "|"))
}

// tagsDueOnMachines checks tags-due on TestTagNextDue's state, with its two
// tags due: each is listed until its tag record, and only on a machine that
// lists the repository; a second tag-due or tag record is refused.
func tagsDueOnMachines(t *testing.T, dir string) {
	t.Helper()
	lines := func(args ...string) []string { return dueLines(t, dir, args...) }
	if got := lines(); len(got) != 2 ||
		!strings.HasPrefix(got[0], "o/here|conformance/v0.1.0-dev.1|"+dueHead+
			"|program/componentization|") {
		t.Fatalf("tags-due = %v", got)
	}
	refused(t, dir, "already recorded", "record", "-worker", "cloud/run", "-kind",
		"tag-due", "-repository", "o/here", "-head", dueHead, "-ref",
		"conformance/v0.1.0-dev.1", "-note", "again")
	must(t, dir, "record", "-worker", "host:x/m.r", "-kind", "tag", "-repository",
		"o/here", "-head", dueHead, "-ref", "conformance/v0.1.0-dev.1", "-note",
		"created by native-tag.sh")
	refused(t, dir, "already recorded", "record", "-worker", "host:x/n.r", "-kind",
		"tag", "-repository", "o/here", "-head", dueHead, "-ref",
		"conformance/v0.1.0-dev.1", "-note", "created again")
	if got := lines(); len(got) != 1 || !strings.HasPrefix(got[0],
		"o/here|relay-claim/v0.2.0-dev.1|") {
		t.Fatalf("tags-due after a tag record = %v", got)
	}

	hosts := "machines:\n  - {name: here, hosts: [qemu-any]}\n" +
		"  - {name: away, hosts: [qemu-any], repositories: [o/there]}\n"
	if err := os.WriteFile(filepath.Join(dir, "hosts.yaml"), []byte(hosts),
		0o644); err != nil {
		t.Fatal(err)
	}
	if got := lines("-machine", "here"); len(got) != 1 {
		t.Errorf("tags-due on a machine that lists o/here = %v", got)
	}
	if got := lines("-machine", "away"); len(got) != 0 {
		t.Errorf("tags-due on a machine that does not list o/here = %v", got)
	}
}
