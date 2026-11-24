package check

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// ghOutEnv carries the output the fake gh prints, and ghHelperEnv marks the
// test binary's run as that fake.
const (
	ghOutEnv    = "WF_FAKE_GH_OUT"
	ghHelperEnv = "WF_FAKE_GH"
)

// TestMain lets the test binary stand in for gh: run with ghHelperEnv set,
// it prints ghOutEnv and exits.
func TestMain(m *testing.M) {
	if os.Getenv(ghHelperEnv) == "1" {
		fmt.Print(os.Getenv(ghOutEnv))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeGH is a ghCLI whose gh prints out, and the argument lists it was run
// with.
func fakeGH(out string) (ghCLI, *[][]string) {
	var calls [][]string
	return ghCLI{command: func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), ghHelperEnv+"=1", ghOutEnv+"="+out)
		return cmd
	}}, &calls
}

// TestGhCLIPR: PR reads the pull request over REST, mergeable_state with it,
// a null one read as unknown, and refuses an output that is not four fields
// (wf-gate-rest Decision 1).
func TestGhCLIPR(t *testing.T) {
	for out, want := range map[string]PRView{
		"program/x abc123 feat/demo clean\n": {"program/x", "abc123", "feat/demo", "clean"},
		"main def456 sync/program-1 unknown": {"main", "def456", "sync/program-1", "unknown"},
	} {
		g, calls := fakeGH(out)
		got, err := g.PR("o/r", 42)
		if err != nil || got != want {
			t.Fatalf("%q: %+v, %v; want %+v", out, got, err, want)
		}
		args := (*calls)[0]
		if args[0] != "gh" || args[1] != "api" || args[2] != "repos/o/r/pulls/42" {
			t.Fatalf("PR ran %v, want gh api repos/o/r/pulls/42", args)
		}
		if jq := args[len(args)-1]; !strings.Contains(jq, `.mergeable_state // "unknown"`) {
			t.Fatalf("PR's --jq %q does not read a null mergeable_state as unknown", jq)
		}
	}
	for _, out := range []string{"", "main abc123 feat/demo", "a b c d e"} {
		g, _ := fakeGH(out)
		if _, err := g.PR("o/r", 42); err == nil ||
			!strings.Contains(err.Error(), "gh api repos/o/r/pulls/42") {
			t.Errorf("%q: %v, want a refusal naming the REST call", out, err)
		}
	}
}

// TestGhCLIRestOnly: no ghCLI method runs gh pr or GraphQL, which Claude
// Code sessions refuse (wf-gate-rest Decision 4).
func TestGhCLIRestOnly(t *testing.T) {
	g, calls := fakeGH("main abc123 feat/demo clean")
	_, _ = g.PR("o/r", 1)
	_, _ = g.CheckRuns("o/r", "abc123")
	_, _ = g.Required("o/r", "main")
	if len(*calls) != 3 {
		t.Fatalf("%d calls, want 3", len(*calls))
	}
	for _, args := range *calls {
		if args[1] != "api" || slices.Contains(args, "graphql") || slices.Contains(args, "pr") {
			t.Errorf("gh %s: not a REST read", strings.Join(args[1:], " "))
		}
	}
}

// loopCommandRe is a gh command the loop's agents and skills must not run:
// gh pr with one of these verbs, or gh api graphql, both GraphQL.
var loopCommandRe = regexp.MustCompile(
	`gh\s+pr\s+(create|ready|merge|view|close|checks|list|edit)\b|gh\s+api\s+graphql\b`)

// TestAgentsUseREST: the loop's agents and skills name no gh pr command and
// no gh api graphql, which Claude Code sessions refuse. A line saying a
// settings file denies the command names one the loop does not run
// (wf-gate-rest Decision 4).
func TestAgentsUseREST(t *testing.T) {
	root := filepath.Join("..", "..", "..", ".claude")
	agents, _ := filepath.Glob(filepath.Join(root, "agents", "*.md"))
	skills, _ := filepath.Glob(filepath.Join(root, "skills", "*", "SKILL.md"))
	files := append(agents, skills...)
	if len(agents) == 0 || len(skills) == 0 {
		t.Fatalf("no agents or skills under %s", root)
	}
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		lines := strings.Split(text, "\n")
		for _, m := range loopCommandRe.FindAllStringIndex(text, -1) {
			n := strings.Count(text[:m[0]], "\n")
			if strings.Contains(lines[n], "denies") {
				continue
			}
			t.Errorf("%s:%d: %q is GraphQL; use its REST form", p, n+1,
				strings.Join(strings.Fields(text[m[0]:m[1]]), " "))
		}
	}
}
