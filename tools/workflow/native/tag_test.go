package native

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// tagRepo is a checkout of o/here whose clone of o/r, at .dev/checkouts/o/r,
// has a bare origin at a path naming o/r with the program trunk program/x at
// in, and the commit out on no branch of it; .dev/checkouts/o/s is a clone
// whose origin names o/r. Its dist/wf is a stub whose tags-due lists the
// tags in due.
type tagRepo struct {
	pushRepo
	clone, rOrigin string
	in, out        string
}

// due lists, as wf tags-due prints them, v0.1.0-dev.1 at in, v0.1.0-dev.2
// at out, v0.1.0-dev.3 at in, which the remote already has, and o/s's.
func (r tagRepo) due() string {
	line := func(repo, tag, sha string) string {
		return fmt.Sprintf("%s\t%s\t%s\tprogram/x\t2026-11-24T18:00:00Z\n", repo, tag, sha)
	}
	return line("o/r", "v0.1.0-dev.1", r.in) + line("o/r", "v0.1.0-dev.2", r.out) +
		line("o/r", "v0.1.0-dev.3", r.in) + line("o/s", "v0.1.0-dev.1", r.in)
}

func newTagRepo(t *testing.T) tagRepo {
	t.Helper()
	r := tagRepo{pushRepo: newPushRepo(t)}
	r.script = filepath.Join(filepath.Dir(r.script), "native-tag.sh")
	r.rOrigin = filepath.Join(filepath.Dir(r.origin), "remote", "o", "r.git")
	r.git(r.dir, "init", "-q", "--bare", r.rOrigin)
	r.clone = filepath.Join(r.dir, ".dev", "checkouts", "o", "r")
	r.git(r.dir, "clone", "-q", r.rOrigin, r.clone)
	for _, kv := range [][2]string{{"user.name", "t"}, {"user.email", "t@t"},
		{"commit.gpgsign", "false"}, {"tag.gpgsign", "false"}} {
		r.git(r.clone, "config", kv[0], kv[1])
	}
	r.git(r.clone, "commit", "-q", "--allow-empty", "-m", "in")
	r.in = r.git(r.clone, "rev-parse", "HEAD")
	r.git(r.clone, "push", "-q", "origin", "HEAD:refs/heads/program/x")
	r.git(r.clone, "commit", "-q", "--allow-empty", "-m", "out")
	r.out = r.git(r.clone, "rev-parse", "HEAD")
	r.git(r.clone, "push", "-q", "origin", r.in+":refs/tags/v0.1.0-dev.3")
	other := filepath.Join(r.dir, ".dev", "checkouts", "o", "s")
	r.git(r.dir, "clone", "-q", r.rOrigin, other)
	r.write("repository: o/here\n", ".dev", "workflow", "config.yaml")
	stub := "#!/bin/sh\n[ \"$1\" = tags-due ] || exit 1\ncat <<'EOF'\n" + r.due() + "EOF\n"
	r.write(stub, "dist", "wf")
	if err := os.Chmod(filepath.Join(r.dir, "dist", "wf"), 0o755); err != nil {
		t.Fatal(err)
	}
	return r
}

// refusedTag runs native-tag.sh and fails unless it refused with one line,
// exit 2 and the text want.
func (r tagRepo) refusedTag(t *testing.T, dir, want string, args ...string) {
	t.Helper()
	out, stderr, code := runLauncher(t, r.script, filepath.Join(r.dir, dir), args)
	if code != 2 || out != "" || strings.Count(stderr, "\n") != 1 ||
		!strings.HasPrefix(stderr, "native-tag.sh: ") || !strings.Contains(stderr, want) {
		t.Fatalf("exit %d, stdout %q, stderr %q; want one refusal line with %q", code,
			out, stderr, want)
	}
}

// TestNativeTag runs native-tag.sh: each refusal is one line and exit 2
// before a tag is created; a due tag is created as an annotated tag at the
// due commit and pushed to the clone's origin; a second run is refused, as
// the remote has it (FLOW-17, D3 of docs/design/program-autonomy.md).
func TestNativeTag(t *testing.T) {
	r := newTagRepo(t)
	for _, c := range []struct {
		name, dir, want string
		args            []string
	}{
		{"no arguments", "", "takes the repository", nil},
		{"one argument", "", "takes the repository", []string{"o/r"}},
		{"an extra argument", "", "takes the repository",
			[]string{"o/r", "v0.1.0-dev.1", "--force"}},
		{"not owner/repo", "", "bad repository", []string{"r", "v0.1.0-dev.1"}},
		{"three parts", "", "bad repository", []string{"o/r/x", "v0.1.0-dev.1"}},
		{"an option as the tag", "", "bad tag", []string{"o/r", "--delete"}},
		{"a refspec as the tag", "", "bad tag", []string{"o/r", "+v1:v2"}},
		{"no clone", "", "not a clone", []string{"o/none", "v0.1.0-dev.1"}},
		{"another repository's clone", "", "not o/s", []string{"o/s", "v0.1.0-dev.1"}},
		{"not due", "", "not due", []string{"o/r", "v9.0.0-dev.1"}},
		{"outside the program trunk", "", "is not in origin/program/x",
			[]string{"o/r", "v0.1.0-dev.2"}},
		{"the remote has it", "", "already has v0.1.0-dev.3 at " + r.in,
			[]string{"o/r", "v0.1.0-dev.3"}},
		{"from a subdirectory", ".dev", "checkout root", []string{"o/r", "v0.1.0-dev.1"}},
	} {
		t.Run(c.name, func(t *testing.T) { r.refusedTag(t, c.dir, c.want, c.args...) })
	}
	if got := r.git(r.dir, "ls-remote", "--tags", r.rOrigin); strings.Contains(got,
		"dev.1") || strings.Contains(got, "dev.2") {
		t.Fatalf("a refusal pushed a tag: %s", got)
	}

	if _, stderr, code := runLauncher(t, r.script, r.dir,
		[]string{"o/r", "v0.1.0-dev.1"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	obj := r.git(r.dir, "--git-dir", r.rOrigin, "rev-parse", "refs/tags/v0.1.0-dev.1")
	if kind := r.git(r.dir, "--git-dir", r.rOrigin, "cat-file", "-t", obj); kind != "tag" {
		t.Fatalf("the tag is a %s, not an annotated tag", kind)
	}
	if at := r.git(r.dir, "--git-dir", r.rOrigin, "rev-parse",
		"refs/tags/v0.1.0-dev.1^{commit}"); at != r.in {
		t.Fatalf("the tag is at %s, want the due commit %s", at, r.in)
	}
	r.refusedTag(t, "", "already has v0.1.0-dev.1 at "+r.in, "o/r", "v0.1.0-dev.1")
}

// TestNativeTickTagsDue runs native-tick.sh's tags_due on gate output: the
// prompt names each tag the gate lists as due, and nothing when it lists
// none (FLOW-17).
func TestNativeTickTagsDue(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	raw, err := os.ReadFile("native-tick.sh")
	if err != nil {
		t.Fatal(err)
	}
	fn := regexp.MustCompile(`(?ms)^tags_due\(\) \{\n.*?^\}\n`).Find(raw)
	if fn == nil {
		t.Fatal("native-tick.sh has no tags_due")
	}
	for _, c := range []struct{ gate, want string }{
		{"step\thost:qemu-any\ta\tf6-validate-qemu-any\ntag\to/r\tv0.1.0-dev.1\n" +
			"tag\to/s\tconformance/v0.2.0-rc.1\nidle\tx",
			" Tags due: o/r v0.1.0-dev.1, o/s conformance/v0.2.0-rc.1."},
		{"step\thost:qemu-any\ta\tf6-validate-qemu-any", ""},
		{"", ""},
	} {
		cmd := exec.Command(bash, "-c", string(fn)+`tags_due "$1"`, "tags_due", c.gate)
		out, err := cmd.Output()
		if err != nil || string(out) != c.want {
			t.Errorf("tags_due on %q = %q, %v; want %q", c.gate, out, err, c.want)
		}
	}
}

// TestNativeTagRule: settings.native.json names the tag script in allow, the
// one route for a due tag, and keeps its push denies (FLOW-17, Q6 of
// docs/design/program-autonomy.md).
func TestNativeTagRule(t *testing.T) {
	raw, err := os.ReadFile("settings.native.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(s.Permissions.Allow, "Bash(tools/workflow/native/native-tag.sh:*)") {
		t.Error("allow lacks Bash(tools/workflow/native/native-tag.sh:*)")
	}
	for _, rule := range []string{"Bash(git push*)", "Bash(git -C * push*)"} {
		if !slices.Contains(s.Permissions.Deny, rule) {
			t.Errorf("deny lacks %s", rule)
		}
	}
}
