package check

import (
	"strings"
	"testing"
)

// mainYML is main.yml's shape: the classifier, two rule-running jobs, an
// ordinary job that only names tier1 and the required ci-ok gate.
const mainYML = `on:
  pull_request:
    types: [opened, synchronize]
jobs:
  changes:
    runs-on: ubuntu-latest
    steps:
      - run: |
          # TestRealIndex (tools/tier1/rules) reads the docs.
          echo code=true >> "$GITHUB_OUTPUT"
  lint:
    needs: changes
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod  # golangci-lint reads this toolchain's export data
      - run: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.2
      - run: golangci-lint run
  tier1:
    needs: changes
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - run: ./dist/tier1 -base "$BASE"
  tier1-test:
    needs: changes
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
        working-directory: tools/tier1
  ci-ok:
    needs: [changes, lint, tier1, tier1-test]
    if: always()
    runs-on: ubuntu-latest
    steps:
      - run: jq -e 'all(.[]; .result == "success")' <<< "$NEEDS"
`

// releaseYML is release.yml's shape: the LINT-03 release gate needs tier1.
const releaseYML = `jobs:
  support-images:
    runs-on: ubuntu-latest
    steps:
      - run: make support-images
  tier1:
    runs-on: ubuntu-latest
    steps:
      - run: ./dist/tier1 -base "$prev"
  release:
    needs: [support-images, tier1]
    runs-on: ubuntu-latest
    steps:
      - run: make release
`

// tidyYML is auto-tidy.yml's shape: make lint runs golangci-lint and tier 1
// without naming them.
const tidyYML = `jobs:
  auto-tidy:
    runs-on: ubuntu-latest
    steps:
      - run: make lint
`

// integrationYML is integration.yml's shape: a job keyed tier1 that runs
// TEST-02's tier 1, not the linter.
const integrationYML = `on:
  pull_request:
    paths: ['relay/**']
jobs:
  tier1:
    runs-on: ubuntu-latest
    steps:
      - run: make integration-test
`

// edit applies old/new replacements to a fixture, failing when one is absent.
func edit(t *testing.T, text string, pairs ...string) string {
	t.Helper()
	for i := 0; i+1 < len(pairs); i += 2 {
		if !strings.Contains(text, pairs[i]) {
			t.Fatalf("fixture lacks %q", pairs[i])
		}
		text = strings.Replace(text, pairs[i], pairs[i+1], 1)
	}
	return text
}

// TestWorkflowGuardChanged: each case edits a fixture on the branch side,
// by old/new pairs, and says whether the edit changes the rule enforcement.
func TestWorkflowGuardChanged(t *testing.T) {
	const needs = "[changes, lint, tier1, tier1-test]"
	keytool := "  keytool:\n    runs-on: ubuntu-latest\n    steps:\n" +
		"      - run: go test ./cmd/warden-io -run Keytool\n  ci-ok:\n"
	block := "\n      - changes\n      - lint\n      - tier1\n      - tier1-test"
	action := "  lint2:\n    runs-on: ubuntu-latest\n    steps:\n" +
		"      - uses: golangci/golangci-lint-action@v8\n  ci-ok:\n"
	report := "  report:\n    needs: tier1\n    runs-on: ubuntu-latest\n" +
		"    steps:\n      - run: echo done\n  ci-ok:\n"
	comment := "          # tier1 and golangci-lint read this output.\n" +
		"          echo code=true"
	aliased := "x: &v 1\njobs:\n  lint:\n    timeout-minutes: *v\n" +
		"    steps:\n      - run: golangci-lint run\n"
	cases := []struct {
		name   string
		before string
		edits  []string
		want   bool
	}{
		{"the incident: keytool added to ci-ok's needs", mainYML, []string{
			"  ci-ok:\n", keytool,
			needs, "[changes, lint, tier1, tier1-test, keytool]"}, false},
		{"ci-ok's needs as a block list", mainYML, []string{needs, block}, false},
		{"a comment edit on lint's setup-go line", mainYML, []string{
			"go.mod  # golangci-lint", "go.mod  # lint"}, false},
		{"tier1 renamed with ci-ok's needs updated", mainYML, []string{
			"  tier1:\n", "  tier1-static:\n",
			"lint, tier1, tier1-test]", "lint, tier1-static, tier1-test]"}, false},
		{"a tier1 step in an ordinary job", mainYML, []string{
			"      - run: go test ./...\n",
			"      - run: ./dist/tier1 -base x\n      - run: go test ./...\n"}, true},
		{"a new job using the golangci-lint action", mainYML, []string{
			"  ci-ok:\n", action}, true},
		{"golangci-lint's version changed in lint", mainYML, []string{
			"golangci-lint@v2.6.2", "golangci-lint@v2.7.0"}, true},
		{"tier1's fetch-depth changed", mainYML, []string{
			"fetch-depth: 0", "fetch-depth: 1"}, true},
		{"tier1 dropped from ci-ok's needs", mainYML, []string{
			"lint, tier1, tier1-test]", "lint, tier1-test]"}, true},
		{"tier1 dropped from release's needs", releaseYML, []string{
			"[support-images, tier1]", "[support-images]"}, true},
		{"support-images dropped from release's needs", releaseYML, []string{
			"[support-images, tier1]", "[tier1]"}, false},
		{"a new job that needs tier1", mainYML, []string{"  ci-ok:\n", report}, false},
		{"a whole-line comment naming tier1 in an ordinary script", mainYML, []string{
			"          echo code=true", comment}, false},
		{"changes' script and the triggers changed", mainYML, []string{
			"echo code=true", "echo code=false",
			"[opened, synchronize]", "[opened, synchronize, labeled]"}, false},
		{"make lint edited in auto-tidy", tidyYML, []string{
			"run: make lint", "run: make lint || true"}, true},
		{"integration's job keyed tier1 and its paths edited", integrationYML, []string{
			"make integration-test", "make integration-test V=1",
			"'relay/**'", "'relay/**', 'drivers/**'"}, false},
		{"an anchor's alias changed in a rule-running job", aliased, []string{
			"&v 1", "&v 2"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			after := edit(t, tc.before, tc.edits...)
			got, err := workflowGuardChanged([]byte(tc.before), []byte(after))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("changed = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestWorkflowGuardAddedDeleted: a missing side has no jobs.
func TestWorkflowGuardAddedDeleted(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after []byte
		want          bool
	}{
		{"a workflow with a rule-running job added", nil, []byte(mainYML), true},
		{"a workflow with a rule-running job deleted", []byte(releaseYML), nil, true},
		{"a workflow with none added", nil, []byte(integrationYML), false},
		{"a workflow with none deleted", []byte(integrationYML), nil, false},
	} {
		got, err := workflowGuardChanged(tc.before, tc.after)
		if err != nil || got != tc.want {
			t.Errorf("%s: changed = %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
}

func TestWorkflowGuardUnparseable(t *testing.T) {
	bad := []byte("jobs:\n  lint: [\n")
	for _, tc := range []struct {
		name          string
		before, after []byte
		where         string
	}{
		{"merge base", bad, []byte(mainYML), "merge base: "},
		{"branch", []byte(mainYML), bad, "branch: "},
		{"jobs not a mapping", []byte(mainYML), []byte("jobs: [lint]\n"),
			"jobs is not a mapping"},
	} {
		if _, err := workflowGuardChanged(tc.before, tc.after); err == nil ||
			!strings.Contains(err.Error(), tc.where) {
			t.Errorf("%s: err = %v, want it to name %q", tc.name, err, tc.where)
		}
	}
}
