---
feature: wf-check-test-windows-eol
status: archived
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required   # process item (FLOW-03): Jeff reviews it at x1-design
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-09, FLOW-14, TEST-01, TEST-02, GLOBAL-10, GLOBAL-11]
rule_changes: []          # process route (FLOW-14); no rule wording changes
concepts: [wf-check, tests, line-endings, windows, autocrlf, squash-merge]
related: [wf-squash-after-base-moved.md]
---

# wf check's test repositories ignore the machine's line-ending setting

## Problem

On 2026-11-12, `make test` on windows-desktop failed in
`tools/workflow/check` (main 1173c5f):

```
cd tools/workflow && go test ./check/ -run TestWhitespaceOnlyDifferenceFails
x3 on a whitespace-only difference: "", want it unmerged
```

`TestWhitespaceOnlyDifferenceFails` (`tools/workflow/check/check_test.go:711-725`,
added by wf-squash-after-base-moved, #107) builds a squash on `main` whose only
difference from the branch is one extra space, by replacing `"Feature line.\n"`
in the checked-out file. The test's repositories are made by the `git` helper
(`check_test.go:97-106`), which runs `git` with the machine's own config. On a
machine with `core.autocrlf=true` (the Windows default), the checkout has CRLF
line endings, the replacement matches nothing, the squash equals the branch, and
the check rightly calls it merged; the test fails.

The test is right about the check; the fixture is not deterministic. Any test
here that edits a checked-out file by its text can break the same way. CI does
not catch it: `main.yml` runs `make test` on ubuntu-latest only. The other two
Windows-only failures from the same run were fixed in #111.

## Prior art

- **wf-squash-after-base-moved** (archived; #107): added this test and the
  patch-id proof it guards. Unchanged here.
- **#111** (b8fb3c4): fixed two other Windows-only test failures, one by making
  the expectation platform-neutral (`filepath.ToSlash`) and one with an
  `eol=lf` attribute for a file that must keep LF. Neither applies: this file is
  a test fixture created at run time, not a checked-in file.
- **`tools/workflow/native/native_test.go:218`** already runs `git` with an
  explicit `-c` config list (`cfg`), the precedent for pinning git config in a
  test helper.

## Decisions

1. **The `git` helper in `check_test.go` pins `core.autocrlf=false` for every
   call,** through the environment (`GIT_CONFIG_COUNT=1`,
   `GIT_CONFIG_KEY_0=core.autocrlf`, `GIT_CONFIG_VALUE_0=false`), so the test
   repositories hold exactly the bytes the tests write, on every machine.
   - **Rejected: change only this test's replacement** to `"Feature line."`
     (no newline). It fixes the one symptom and leaves every other text edit in
     the package tied to the machine's setting.
   - **Rejected: `git config core.autocrlf false` in `repoWith`.** It covers
     the clone made there but not `git` calls that run elsewhere before it (the
     bare `init`), and is one more step each new fixture helper must remember.
   - **Rejected: a `.gitattributes` in each test repository.** It needs a commit
     in every fixture and changes the trees some tests compare.
2. **The same pin goes into the other test git helpers in `tools/workflow`**
   (`queue/hosts_test.go:19`, `native/native_test.go:218`), so no package's
   fixtures depend on the machine's line endings (question a1).
3. **No CI change here.** Running `tools/workflow`'s tests on windows-latest is
   a CI cost question for its own item (question a2).

## Questions

### Blocking

None.

### Assumed

- **a1. Scope** (Decision 2). Proceeding with all three test git helpers in
  `tools/workflow`. Options: (a) as stated; (b) `check_test.go` only, the one
  package that fails today. Recommended: (a).
- **a2. CI** (Decision 3). Proceeding with no CI change. Options: (a) as
  stated; (b) also run `(cd tools/workflow && go test ./...)` on windows-latest
  in `main.yml`'s `tier1-test` matrix, which already runs on windows for
  `tools/tier1` (a rule-running job under FLOW-14, so a process-item change).
  Recommended: (a), filed as a follow-up only if you want it.

## Design

`tools/workflow/check/check_test.go`, `git` (`:97-106`): `cmd.Env` gains
`"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.autocrlf",
"GIT_CONFIG_VALUE_0=false"`. The two `exec.Command("git", ...)` calls that read
output (`:117`, `:695`, `:758`) read only, so they need no pin.
`queue/hosts_test.go:19` and `native/native_test.go:218` get the same three
variables. No production code changes.

## Defaults and overrides

None. The pin is a test-fixture setting; it changes no default, override, merge
or fallback in `wf`.

## Cross-driver impact

None. No driver or product code changes.

## Security note

`security_boundary: none`. Test code only.

## Validation plan

- **Tier 0:** `(cd tools/workflow && go test ./...)` passes on linux. A new
  test, `TestGitHelperIgnoresAutoCRLF` in `check_test.go`, sets
  `core.autocrlf=true` in a temporary global config (`GIT_CONFIG_GLOBAL`),
  writes an LF file through the helper's repo, checks it out again and asserts
  the bytes are unchanged; it fails without the pin.
- **On windows-desktop:** `make test` passes, `TestWhitespaceOnlyDifferenceFails`
  included (a check on the next native run, not a blocking step).
- No native validation steps. No tier-2 gaps.

## Implementation plan

Confirmed at x2-implement (process item, FLOW-03), one commit each.

1. `tools/workflow/check/check_test.go`: the pin in `git`, and
   `TestGitHelperIgnoresAutoCRLF` (TEST-01, TEST-02).
2. `tools/workflow/queue/hosts_test.go`, `tools/workflow/native/native_test.go`:
   the same pin (a1).
3. Gates: `make lint`, `make test`, `make enforcement` (FLOW-14),
   `tools/workflow/flow18-check.sh`.

## Design review

- **r1, 2026-11-12: approved at `d9c8e99`** (Jeff, workflow-human). Assumptions a1 (all three test git helpers) and a2 (no CI change) confirmed as drafted, (a) each. No changes folded in. No `rule_changes` (process item, FLOW-14).

## Consolidation

Branched from `origin/main` at `b8fb3c4` (#111), still `main` when Implement
finished: nothing landed to reconcile.

- **FLOW-15 sweep:** test helpers only. `overview.md` describes the product
  components, not `tools/workflow`, and is unchanged. No user-facing change, so
  no reference docs (UX-01).
- **Pins:** `check_test.go`'s `git` and `queue/hosts_test.go`'s `gitRepo` set
  `GIT_CONFIG_COUNT=1`, `GIT_CONFIG_KEY_0=core.autocrlf`,
  `GIT_CONFIG_VALUE_0=false` beside their other variables;
  `native_test.go`'s `newLauncherRepo` adds `-c core.autocrlf=false` to its
  `cfg` list, matching that helper's style (Decision 2). The reads that call
  `exec.Command("git", ...)` directly are unchanged.
- **Tests:** `TestGitHelperIgnoresAutoCRLF` added. With the pin removed it
  fails (`checked-out bytes "one\r\ntwo\r\n", want "one\ntwo\n"`).
  `GIT_CONFIG_GLOBAL` set to a file with `core.autocrlf = true`, `go test
  ./...` in `tools/workflow` fails at `98fffd8` (before task 1)
  (`TestWhitespaceOnlyDifferenceFails` in `check`, and `TestNativeRun` in
  `native`, a second case the same setting broke) and passes on the branch.
- **Gates:** `make lint` (0 findings), `make test`, `make enforcement` ("main's
  tier 1 findings kept", 0 either side) and `tools/workflow/flow18-check.sh`
  pass.

## Rollback notes

None yet.

## Archive summary

Approved at design review r1 (2026-11-12, Jeff); implemented and published
the same day as a process item. The PR to `main` carries this design.

- **Built.** The three test git helpers in `tools/workflow` pin
  `core.autocrlf=false`: `check_test.go`'s `git` and `queue/hosts_test.go`'s
  `gitRepo` through `GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_0`/`GIT_CONFIG_VALUE_0`
  in the environment, `native_test.go`'s `newLauncherRepo` with
  `-c core.autocrlf=false` in its `cfg` list. `TestGitHelperIgnoresAutoCRLF`
  sets `core.autocrlf=true` in a temporary global config and asserts a
  checked-out LF file keeps its bytes. With `core.autocrlf=true` globally,
  `TestWhitespaceOnlyDifferenceFails` and `TestNativeRun` fail before the
  change and pass after it. No production code changes.
- **Decisions taken:** pin the setting in each helper's every `git` call,
  so fixtures hold exactly the bytes the tests write (Decision 1); all three
  helpers, not only `check`'s (a1); no CI change (a2).
- **Rejected:** editing only the failing test's replacement string; `git
  config core.autocrlf false` inside `repoWith` (misses calls before it); a
  `.gitattributes` in each fixture repository (an extra commit that changes
  compared trees); running `tools/workflow`'s tests on windows-latest in CI
  (left for its own item if wanted).
- **For a future design touching `wf-check`, `tests`, `line-endings` or
  `autocrlf`:** a new test git helper in `tools/workflow` must pin
  `core.autocrlf=false` the same way, or its fixtures depend on the machine.
  Read-only `exec.Command("git", ...)` calls need no pin. CI still runs these
  tests on linux only, so a Windows-only fixture break shows up first on
  windows-desktop's `make test`.
