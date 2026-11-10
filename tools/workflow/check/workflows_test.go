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

func TestWorkflowGuardChanged(t *testing.T) {
	keytool := "  keytool:\n    runs-on: ubuntu-latest\n    steps:\n" +
		"      - run: go test ./cmd/warden-io -run Keytool\n  ci-ok:\n"
	cases := []struct {
		name          string
		before, after *string
		want          bool
	}{
		{"the incident: keytool added to ci-ok's needs", side(mainYML), side(edit(t, mainYML,
			"  ci-ok:\n", keytool,
			"[changes, lint, tier1, tier1-test]", "[changes, lint, tier1, tier1-test, keytool]")),
			false},
		{"ci-ok's needs as a block list", side(mainYML), side(edit(t, mainYML,
			"[changes, lint, tier1, tier1-test]",
			"\n      - changes\n      - lint\n      - tier1\n      - tier1-test")), false},
		{"a comment edit on lint's setup-go line", side(mainYML), side(edit(t, mainYML,
			"go-version-file: go.mod  # golangci-lint", "go-version-file: go.mod  # lint")),
			false},
		{"tier1 renamed with ci-ok's needs updated", side(mainYML), side(edit(t, mainYML,
			"  tier1:\n", "  tier1-static:\n",
			"lint, tier1, tier1-test]", "lint, tier1-static, tier1-test]")), false},
		{"a tier1 step in an ordinary job", side(mainYML), side(edit(t, mainYML,
			"      - run: go test ./...\n", "      - run: ./dist/tier1 -base x\n"+
				"      - run: go test ./...\n")), true},
		{"a new job using the golangci-lint action", side(mainYML), side(edit(t, mainYML,
			"  ci-ok:\n", "  lint2:\n    runs-on: ubuntu-latest\n    steps:\n"+
				"      - uses: golangci/golangci-lint-action@v8\n  ci-ok:\n")), true},
		{"golangci-lint's version changed in lint", side(mainYML), side(edit(t, mainYML,
			"golangci-lint@v2.6.2", "golangci-lint@v2.7.0")), true},
		{"tier1's fetch-depth changed", side(mainYML), side(edit(t, mainYML,
			"fetch-depth: 0", "fetch-depth: 1")), true},
		{"tier1 dropped from ci-ok's needs", side(mainYML), side(edit(t, mainYML,
			"lint, tier1, tier1-test]", "lint, tier1-test]")), true},
		{"tier1 dropped from release's needs", side(releaseYML), side(edit(t, releaseYML,
			"[support-images, tier1]", "[support-images]")), true},
		{"support-images dropped from release's needs", side(releaseYML),
			side(edit(t, releaseYML, "[support-images, tier1]", "[tier1]")), false},
		{"a new job that needs tier1", side(mainYML), side(edit(t, mainYML,
			"  ci-ok:\n", "  report:\n    needs: tier1\n    runs-on: ubuntu-latest\n"+
				"    steps:\n      - run: echo done\n  ci-ok:\n")), false},
		{"a whole-line comment naming tier1 in an ordinary script", side(mainYML),
			side(edit(t, mainYML, "          echo code=true",
				"          # tier1 and golangci-lint read this output.\n          echo code=true")),
			false},
		{"changes' script and the triggers changed", side(mainYML), side(edit(t, mainYML,
			"echo code=true", "echo code=false",
			"[opened, synchronize]", "[opened, synchronize, labeled]")), false},
		{"make lint edited in auto-tidy", side(tidyYML), side(edit(t, tidyYML,
			"run: make lint", "run: make lint || true")), true},
		{"integration's job keyed tier1 and its paths edited", side(integrationYML),
			side(edit(t, integrationYML, "make integration-test", "make integration-test V=1",
				"'relay/**'", "'relay/**', 'drivers/**'")), false},
		{"a workflow with a rule-running job added", nil, side(mainYML), true},
		{"a workflow with a rule-running job deleted", side(releaseYML), nil, true},
		{"a workflow with no rule-running job added", nil, side(integrationYML), false},
		{"a workflow with no rule-running job deleted", side(integrationYML), nil, false},
		{"an anchor's alias changed in a rule-running job", side("x: &v 1\njobs:\n" +
			"  lint:\n    timeout-minutes: *v\n    steps:\n      - run: golangci-lint run\n"),
			side("x: &v 2\njobs:\n  lint:\n    timeout-minutes: *v\n    steps:\n" +
				"      - run: golangci-lint run\n"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := workflowGuardChanged(text(tc.before), text(tc.after))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("changed = %v, want %v", got, tc.want)
			}
		})
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

func side(s string) *string { return &s }

func text(s *string) []byte {
	if s == nil {
		return nil
	}
	return []byte(*s)
}
