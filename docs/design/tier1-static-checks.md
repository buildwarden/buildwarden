---
feature: tier1-static-checks
status: archived
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [LINT-01, LINT-02, LINT-03, LINT-04, LINT-05, LINT-06, LINT-07, LINT-08, GLOBAL-01, GLOBAL-02, GLOBAL-03, GLOBAL-04, GLOBAL-07, GLOBAL-08, EXEC-01, EXEC-04, EXEC-05, RELAY-01, RELAY-05, RELAY-07, ORCH-01, ORCH-03, UX-01, UX-02, UX-03, UX-04, BOOT-07, FLOW-01, FLOW-02, FLOW-03, FLOW-08, FLOW-12, FLOW-14, FLOW-15, TEST-01, TEST-02]
concepts: [behavioral-linter, tier1, static-checks, go-analysis, diff-coupling, suppressions, rule-index, release-gate, design-front-matter]
related: [workflow-bootstrap.md]
---

# Tier-1 static-check extraction (LINT-02, Deep-dive #2)

## Problem

LINT-02 splits the behavioral linter into a deterministic tier 1 and a
judgment tier 2, and says tier 1 includes "custom static checks for the
`automated`-tagged rules". None exist. Tier 1 today is `make lint`
(golangci-lint with six linters) plus `make test`. Every review report so far
records "Custom static checks (LINT-02): none exist yet"
(`reports/container-inputs/3a7a1b36e0e1.md`,
`reports/design-front-matter-retrofit/7b11f043ab61.md` on the `workflow`
branch).

The cost shows up in those reports. Tier 2 spends its findings on mechanical
facts that a script could settle: whether a design's front-matter is complete
and consistent, whether `docs/` moved with a surface, and whether the `drivers`
claim matches the trees a diff reaches. These are reported as advisory MUST
findings and cost a return to Design each time, when a deterministic check
would have failed the change before review. LINT-03 also wants tier 1 to be the
hard-fail part of the release gate, and today it has nothing beyond lint and
tests to fail on.

This matters now because the workflow loop is running. Its Review gate is
"tier 1 green, tier-2 report present" (`workflow-bootstrap.md`, Decision
"Review is report-only"), so every check added here makes the gate
deterministic for each feature after this one.

## Prior art

- **`workflow-bootstrap.md`** (active). This design reuses three of its
  choices. Tooling goes in its own Go module under `tools/`, so the product
  module gets no new dependencies (its `wf` decision). Done-when checks run
  `make lint` and `make test` (`tools/workflow/check`, phases 3 and 4). Review
  is report-only for tier 2 (LINT-07). Its priority question already ranks this
  feature as the one that "makes the Review gate deterministic for everything
  after it".
- **`tools/workflow/check`** (on `main`). It already parses design front-matter
  (`ParseDesign`) and validates FLOW-02 keys and values (`Front.problems`,
  unexported). That logic is the obvious source for the archive-wide
  front-matter check. See Questions a2 and a3.
- **`design-front-matter-retrofit`** (queued, not on `main`). Its open
  assumption a8 proposes handing "a permanent archive-wide front-matter check"
  to this feature. That assumption is unconfirmed, so it is treated as a
  candidate input (Question a2), not a requirement.
- **`unified-bootstrap.md`** (archived) and the `Makefile`'s `deadcode` target.
  `make deadcode` is the FLOW-15 sweep. It is whole-repo and run at Publish
  consolidation, so it stays outside the diff-scoped tier 1 (LINT-04) and is not
  duplicated here.
- **Standards.** LINT-01 (the extractable rule index), LINT-02 (two tiers,
  every rule mapped), LINT-03 (tier 1 hard-fails), LINT-04 (diff-scoped),
  LINT-05 (stable output keyed by rule and `file:line`, suppressible with a
  reason), LINT-06 (the agent is handed the extracted rule index), LINT-08 (a
  rule's `Check:` line is its linter spec). Each `automated` / hybrid rule's
  "The automated part ..." sentence is the specification each check below
  implements; this design does not restate them.

No archived design covers linting. No pre-standard design without
front-matter is relied on, so FLOW-12 requires no retrofit here.

## Decisions

1. **A tool of its own, `tier1`, in its own module at `tools/tier1/`.** It has
   its own `go.mod`, and its dependencies are `golang.org/x/tools` for
   `go/analysis` and a YAML library. It reads the root module's packages
   through `go/packages` and imports none of them. Rejected:
   - A package under `tools/workflow/`. That would share the YAML dependency
     and the front-matter parser, but it is the queue tool and is off-limits
     to unattended edits (FLOW-14). Tier 1 also runs in CI and at release,
     where no queue exists.
   - A package in the product module. That would add `x/tools` and YAML to the
     module third parties embed; `workflow-bootstrap` rejected this for the
     same reason.
   - A golangci-lint module plugin. That needs a custom golangci-lint build in
     every session and CI job, and it cannot run the git-diff coupling checks.

   Confirm as a1.
2. **Three mechanisms, chosen by what the check needs to see.** This is
   LINT-02's open choice.
   - **Coupling checks** need only the diff's file list and, for some checks,
     the design front-matter. Example: a changed path in set A with no changed
     path in set B.
   - **Source checks** need Go syntax or types. They run as `go/analysis`
     passes through `golang.org/x/tools/go/analysis/checker` over the packages
     that contain changed files.
   - **Existing linters** cover rules a stock golangci-lint linter already
     expresses. A second config, `tools/tier1/golangci.yml`, runs with
     `--new-from-merge-base`, so it is diff-scoped without custom code.

   Rejected: ripgrep scripts. `rg` is not guaranteed on every host (GLOBAL-07,
   Windows contributors), and a text match cannot tell a call from a comment.
   The "ripgrep-shaped" checks become `go/analysis` passes or Go path matching.
3. **Diff-scoped by construction** (LINT-04). The base is `-base <rev>`, and
   defaults to `git merge-base origin/main HEAD`. Line-level findings
   (analyzers, linters) are kept only when they fall on a line the diff added
   or changed. File-level findings (coupling) look only at changed paths.
   - CI on a PR passes the PR base SHA.
   - CI on a push to `main` passes `github.event.before`.
   - The release job passes the previous `v*` tag.

   An unresolvable base fails with the command that fixes it (UX-04), for
   example `git fetch origin main`. It never silently falls back to a
   whole-repo run.
4. **Rule severity decides the exit code** (LINT-03).
   - An unsuppressed finding against a MUST or MUST-review rule fails tier 1.
   - A finding against a SHOULD rule is printed and exits 0.
   - A mechanical signal too noisy to block is emitted as a **hint**. A hint
     never fails; it is listed in the JSON for tier 2 to judge. Today only
     GLOBAL-04 (non-test code changed in a package with no test change) is a
     hint.

   Confirm as a6.
5. **Suppression by commit trailer** (LINT-05). The form is
   `Tier1-Allow: <RULE-ID> <path-glob>: <reason>` in any commit in the
   diff's range. It needs no file, it lives in history, and it falls out of
   scope as soon as the diff no longer contains that commit. A trailer with no
   reason is rejected. Every applied suppression is printed in its own section
   of the output, so the tier-2 report and the human sign-off see it (FLOW-08).
   This means an unattended agent cannot silence a check without the human
   seeing that it did. Rejected: a suppressions file (churn, and stale entries
   outlive the change) and inline `//nolint`-style comments (they cannot mark
   file-level coupling findings). Confirm as a5.
6. **Designs in the diff are the claim.** Some checks compare the change
   against its design's front-matter: ORCH-01 `drivers`, BOOT-07 `rules`, and
   GLOBAL-08 `seams`. They read the union over every `docs/design/*.md` the
   diff adds or modifies, `TEMPLATE.md` excepted.
   - A per-feature diff therefore uses that feature's design.
   - A release diff uses all the designs that landed in it.

   If code changed and no design is in the diff, the run reports a FLOW-01
   finding. The fix is `-no-design`, which CI passes when the PR carries the
   `no-design` label, or a `Tier1-Allow: FLOW-01` trailer.
7. **The rule index is tier 1's own input and output** (LINT-01, LINT-02,
   LINT-06).
   - `tier1 rules` parses `docs/development/*.md` headers and their
     `Severity` / `Check` lines into JSON, which is the index tier 2 is handed.
   - `tools/tier1/rules.yaml` maps every rule whose check type includes
     `automated` to one of: a tier-1 check ID; an external owner (`wf`,
     `flow18-check.sh`, `make test`, CI matrix, branch protection); or
     `follow-up` with a one-line reason.
   - A self-check fails tier 1 when an `automated`-tagged rule has no mapping,
     when a mapping names a rule that no longer exists, or when a header lacks
     a valid `Severity` or `Check` line.

   This makes "every rule mapped to one tier" a mechanical fact instead of a
   standing TODO.
8. **Wired in through `make lint`, CI, and the release workflow.**
   - `make lint` gains a `tier1` run (built to `dist/tier1`). The workflow's
     phase-3 and phase-4 done-when checks already run `make lint`, so the
     Review gate picks tier 1 up with no edit to `tools/workflow/`.
   - `.github/workflows/main.yml` gains a `tier1` job, checked out with
     `fetch-depth: 0`, that runs `tier1 -json` against the PR base and uploads
     the JSON.
   - `.github/workflows/release.yml` gains a job that runs tier 1 against the
     previous tag. The release jobs need it, which gives LINT-03's release
     gate a concrete home.

   Rejected: a separate `make tier1` run only by a new `wf` check, which would
   need a `tools/workflow` edit (FLOW-14). Confirm as a4.
9. **First slice now, the rest mapped as follow-up** (scope; confirm as a7).
   The checks below ship in this feature. Every other `automated` rule gets a
   `rules.yaml` entry, either owned elsewhere or `follow-up` with its reason.
   Sign-off then queues one follow-up item for the remaining checks.

## Questions

### Blocking

None. The one decision that reaches `tools/workflow/` is the shared
front-matter validator (a3). It is settled as an assumption whose fallback
needs no edit there, so nothing stops planning.

### Assumed

- **a1. Home.** Proceeding with a separate module at `tools/tier1/` (Decision
  1); confirm.
  - Options:
    - `tools/tier1/` as its own module.
    - A package inside `tools/workflow/`. This needs Jeff's attended edits
      (FLOW-14), and tier 1 would be tied to the queue tool.
    - `internal/tier1` in the product module. This adds `x/tools` and YAML
      dependencies to the module third parties embed.
  - **Recommended:** `tools/tier1/` as its own module.
- **a2. design-front-matter-retrofit a8.** Proceeding with accepting the
  handoff in a diff-scoped form; confirm.
  - The per-diff run validates the FLOW-02 front-matter of every design the
    diff touches. That covers every new design and every edit to an old one.
  - `tier1 designs -all` checks the whole archive. The release job runs it,
    and so can a human on demand. It is not per-PR, because LINT-04 forbids
    whole-repo runs there.
  - Options:
    - The diff-scoped check plus `-all` at release.
    - An archive-wide check on every PR. This contradicts LINT-04, so it
      would need a rule change.
    - Decline the handoff and leave the check to `wf`'s phase-1 gate, which
      covers only the feature's own design.
  - **Recommended:** the diff-scoped check plus `-all` at release. It holds
    whether or not a8 is confirmed: if a8 picks "no permanent check", the
    `-all` mode is dropped and the diff-scoped check stays.
- **a3. One front-matter validator, not two** (FLOW-15). Proceeding with
  `tier1` importing `tools/workflow/check` through a `replace` directive and
  calling its exported `ParseDesign`; confirm.
  - The field rules live in the unexported `Front.problems`. Exporting it
    (`Problems`, a rename plus its callers) is a `tools/workflow` edit, so the
    plan lists it as a `human`-affinity task, `p3-export-problems`, for an
    attended session.
  - Until that task is done, the front-matter check is not built and the
    other checks proceed.
  - Options:
    - The attended export, then share the validator.
    - Duplicate the ~30 lines of key and enum rules in `tier1`, with a fixture
      test pinning that both accept and reject the same documents.
    - Move the parser into a third shared module. This means more churn and a
      `tools/workflow` edit anyway.
  - **Recommended:** the attended export. If Jeff declines it, fall back to
    the duplicate with the parity test.
- **a4. Wiring.** Proceeding with tier 1 inside `make lint`, a CI job on PRs
  and pushes to `main`, and a release-workflow job against the previous tag
  that the release jobs `need` (Decision 8); confirm.
  - Options:
    - Inside `make lint`, plus CI and the release job.
    - A separate `make tier1` target that `wf` runs. This needs a
      `tools/workflow` edit.
    - CI only. Then the loop's Review gate never sees it.
  - **Recommended:** inside `make lint`, plus CI and the release job.
  - Note: once this merges, `make lint` fails on in-flight branches whose
    diffs violate a MUST check. Their Publish consolidation (FLOW-10) is
    where that is paid.
- **a5. Suppressions.** Proceeding with `Tier1-Allow:` commit trailers
  (Decision 5), each printed in the output for the tier-2 report and sign-off;
  confirm.
  - Options:
    - Commit trailers.
    - A checked-in suppressions file.
    - Inline comments, for line findings only.
    - No suppressions in tier 1. This contradicts LINT-05.
  - **Recommended:** commit trailers.
- **a6. Hints.** Proceeding with GLOBAL-04's mechanical half emitted as a
  non-failing hint for tier 2 rather than a hard failure (Decision 4); confirm.
  - The signal "code changed, no test changed" is wrong for refactors and
    renames often enough that blocking on it would teach agents to write
    suppressions. That "noisy revolt" is the one LINT-07 warns about.
  - Options:
    - A hint.
    - A MUST failure that a suppression can clear.
    - Leave GLOBAL-04 entirely to tier 2.
  - **Recommended:** a hint.
- **a7. Scope.** Proceeding with the first slice in the Design section below.
  The remaining `automated` rules are recorded in `rules.yaml` as owned
  elsewhere or `follow-up`, and sign-off queues one follow-up item; confirm.
  - Options:
    - This slice.
    - Every `automated` rule in this feature. This is well over the budget of
      6 runs and 30 steps, and some rules have no mechanical form yet.
    - The coupling checks only.
  - **Recommended:** this slice.

## Design

### Command

```
tier1 [-base REV] [-json] [-design PATH]... [-no-design] [-repo DIR]
tier1 rules [-json]                 # the extracted rule index (LINT-01, LINT-06)
tier1 designs [-all]                # FLOW-02 front-matter of touched (or all) designs
```

The default run computes the diff `REV...HEAD` and then runs four stages:

1. **Self-check.** The rule index and the `rules.yaml` mapping. This stage
   runs only when `docs/development/` or `rules.yaml` changed, and with
   `-all`.
2. **Coupling checks.**
3. **Analyzers** over the changed packages, built for the host `GOOS`.
   Developers on other platforms run `GOOS=<os> make lint`, as FLOW-17
   already asks.
4. **The golangci-lint tier-1 config.**

Findings are then filtered to the diff, suppressions are applied, and the run
reports.

Text output is grouped by severity (UX-02). JSON is a stable schema keyed by
rule ID and `file:line`:

```
{base, head, findings:[{rule, severity, file, line, check, message}],
 hints:[...], suppressed:[{rule, path, reason, commit}], counts:{...}}
```

Its schema file lives beside the tool, and the tier-2 report embeds it.

Exit codes:

| Code | Meaning |
|---|---|
| 0 | Clean, or SHOULD findings and hints only |
| 1 | Unsuppressed MUST or MUST-review findings |
| 2 | Usage or environment error, for example an unresolvable base |

### Checks in this feature

| Rule | Check id | Mechanism | What it flags (the rule's "automated part") |
|---|---|---|---|
| LINT-01, LINT-02 | `rule-index` | self-check | A rule header without a valid `Severity` / `Check` line; an `automated`-tagged rule with no `rules.yaml` mapping; a mapping to a rule that no longer exists. |
| FLOW-01, FLOW-02, FLOW-12 | `design-front-matter` | coupling + parse | Code changed and no design in the diff (unless `-no-design`); a touched design whose front-matter fails the FLOW-02 validator (a3). |
| ORCH-01, FLOW-02 | `drivers-claim` | coupling | The diff reaches `driver/<d>/` but `<d>` is not in the designs' `drivers`. It also reaches a shared package (`driver/*.go`, `relay/`, `ledger/`, `collector/`, `relaycfg/`, `bootstrap/`) without every driver declared, per the TEST-02 caveat. |
| BOOT-07 | `bootstrap-surface` | coupling | The diff touches the surface BOOT-07 lists without touching `docs/development/bootstrap.md`; or it touches `bootstrap.md` with no design in the diff whose `rules` cites BOOT-07. |
| RELAY-05 | `schema-doc` | coupling | A change under `schemas/` with no change to `docs/book/src/design/ledger-spec.md`, or the reverse. The path pairs are data in `rules.yaml`, so `specs-migration` updates one table. |
| GLOBAL-08, EXEC-04 | `seam-contract` | coupling | A change to a versioned seam's code (`relaycfg/`, `collector/` protocol files, the executor's `initialize` contract) with no change to that seam's contract document. Seams with no contract document yet are mapped `follow-up` rather than failing every change. |
| UX-01, ORCH-03 | `surface-docs` | source + coupling | The set of `cobra` flag names under `cmd/warden/` and `toml:` keys in its config types differs between base and head, and neither `docs/book/src/guide/cli.md` nor `configuration.md` changed. |
| UX-03 | `surface-removed` | source | A flag or config key present at base and absent at head with no alias or `Deprecated` marking. This shares its extractor with `surface-docs`. |
| RELAY-01 | `ledger-single-writer` | go/analysis | A call to the ledger's append or encode path in `relay/` from any function other than `(*Ledger).loop`. |
| RELAY-07 | `relay-diskless` | go/analysis | A call to an `os` / `ioutil` file-creating or file-writing function in non-test files under `relay/`. |
| EXEC-01 | `executor-no-cgo` | go/analysis | `import "C"` in a package under `cmd/warden-io/`. |
| EXEC-05 | `large-literal` | go/analysis | A string literal over 4 KiB under `cmd/warden-io/`. SHOULD, so the finding is informational. |
| GLOBAL-01 | `exported-doc` | golangci (`revive` `exported`, stutter check off) | A new exported identifier with no doc comment, on changed lines only. |
| GLOBAL-02 | `gofmt-touched` | golangci formatter `gofmt -s` | Touched lines that are not gofmt-clean. SHOULD. |
| GLOBAL-03 | `error-wrap` | golangci (`errorlint` errorf; `forbidigo` `panic` outside `main` and test packages) | A `fmt.Errorf` with an error argument and no `%w`; a new `panic` in a library path. |
| GLOBAL-04 | `tests-with-change` | coupling, **hint** (a6) | A package's non-test `.go` files changed and none of its `_test.go` files did. |

### Mapped elsewhere (no new check)

| Owner | Rules |
|---|---|
| `wf` (`tools/workflow/check`, the queue) | FLOW-03, FLOW-04, FLOW-05, FLOW-13, FLOW-16 (the merged-branch report) |
| `flow18-check.sh` | FLOW-18 |
| `make test` / CI matrix | TEST-01, GLOBAL-07, ORCH-05, EXEC-01's cross-build half, TEST-05 (round-trip tests exist as tests) |
| Branch protection, PR checks | FLOW-08, FLOW-11, LINT-03 (through the release job in Decision 8) |
| `make deadcode`, Publish consolidation | FLOW-15 (whole-repo by design, not diff-scoped) |
| Tier 1 itself | LINT-04, LINT-05 (properties of this tool) |
| Security suite (`security-suite` item) | TEST-08, SEC-02, SEC-06, SEC-08 |

### Follow-up (mapped `follow-up` in `rules.yaml`)

- **ORCH-02.** "Input mutations land under `inputs/`" needs a model of which
  writes are input mutations, which only `driver/session.go`'s
  `RecordInputs` defines today.
- **BOOT-04.** "Fetch paths are the only readers of build inputs" needs the
  set of input paths named in code.
- **EXEC-03.** `platform_*.go` coverage for the trust path.
- **UX-02.** TTY detection and a structured mode per command.
- **UX-07, UX-08, UX-09.** These are tests, not static checks: defaults with
  no config, validation at startup, and exit-code tables.
- **RELAY-02.** The round-trip test exists. A check that a new serving path
  is covered by it is follow-up.
- **RELAY-06.** A benchmark threshold, which needs the benchmark in CI.
- **TEST-02, TEST-03, TEST-07, FLOW-09.** Scoping and `tier2_gaps`
  consistency.
- **GLOBAL-08 seams without a contract document.**

Some of these may have no mechanical form at all. The follow-up item proposes
sharpening or re-tagging those rules (LINT-08) for Jeff's sign-off rather than
editing `docs/development/` (FLOW-14).

### Layout

```
tools/tier1/
  go.mod                  module github.com/buildwarden/buildwarden/tools/tier1
  cmd/tier1/main.go       flags, output, exit codes
  diff/                   base resolution, changed files and line ranges, trailers
  rules/                  rule-index parser, rules.yaml loader, self-check
  coupling/               path-set checks; design front-matter union
  analyzers/              one go/analysis pass per source check, with analysistest data
  surface/                cobra-flag and toml-key extractor shared by surface-docs / surface-removed
  golangci.yml            the diff-scoped tier-1 golangci-lint config
  rules.yaml              rule -> check | owner | follow-up
  report.schema.json      the JSON output schema
```

## Cross-driver impact

None. No driver, seam or user-facing `warden` surface changes. Tier 1 reads
`driver/*` paths to compare them with a design's `drivers` claim, but it
changes no driver code. Every driver's future changes are checked the same
way (ORCH-01).

## Validation plan

- **Tier 0.** `go test ./...` in `tools/tier1`.
  - Each analyzer is tested with `analysistest` testdata, with positive and
    negative cases.
  - Each coupling check and the trailer parser are tested with table tests
    over a scratch git repository, the same technique as
    `tools/workflow/check/check_test.go`.
  - The rule-index self-check runs against the real `docs/development/`.
  - The `surface` extractor is tested against a fixture `cobra` command tree.
- **Platforms (GLOBAL-07).** `tools/tier1` tests run in CI on `ubuntu-latest`,
  `macos-latest` and `windows-latest`. The paths it compares come from git,
  which always uses `/`, and it never joins them with the OS separator. No
  native host is needed, so `native_validation` and `tier2_gaps` are empty.
- **Against history.** Before wiring tier 1 into `make lint`, run it on the
  diffs of the last several merged PRs (#60 to #64) and on both in-flight
  branches. Record the findings and the false positives in the Implementation
  plan's notes. A check that is noisy on known-good history is retuned or
  demoted to a hint before it blocks (LINT-07's spirit for tier 1).
- **The loop.** The first feature reviewed after merge exercises the
  phase-3 and phase-4 gates through `make lint`.

## Implementation plan

Phase 2 (p2-plan). This plan expands Design's ten-step order into tasks.
Every task names the rules it must satisfy and the assumed questions it rests
on. All of a1 to a7 are still open, and a different answer changes only the
tasks that name it. "Test tier 0" means TEST-02's unit tier: pure-Go,
table-driven, and run by `make test` on every platform (TEST-01). No task
here needs test tier 1 or 2.

**Steps.** Three phase-3 steps carry the tasks, and the queue file lists the
two that are not template steps. `p3-implement` (cloud, the gate) carries
tasks 1 to 8 and 11 to 13, and may take several claims.
`p3-export-problems` (human, a3) carries task 9. It is attended only, because
unattended runs do not edit `tools/workflow/` (FLOW-14). It has no `needs`, so
Jeff can take it at any point, even before phase 3 starts.
`p3-front-matter-check` (cloud, `needs: [p3-export-problems]`) carries task
10.

No cloud task waits on the human step except task 10. The design union and
the FLOW-01 "no design in the diff" finding need only the already-exported
`check.ParseDesign`, so they are in task 4 rather than task 10.

`wf` advances phase 3 only when all three steps are done. It runs the gate
check (`make lint`, `make test`, docs touched) only when `p3-implement` is
marked done. Mark `p3-implement` done once its own tasks are finished, even
if task 10 is still waiting; releasing it and claiming it again each run
would spend budget for nothing. Task 10 runs `make lint` and `make test`
itself before its `wf done`, and phase 4's gate runs both again on the final
tree.

**Rework of existing code** (FLOW-06): task 9 (`tools/workflow/check`,
attended), task 12 (`Makefile`, `.github/workflows/main.yml`,
`.github/workflows/release.yml`) and task 13 (`CONTRIBUTING.md`,
`docs/book/src/contributing/structure.md`). Everything else is new code under
`tools/tier1/`.

**`overview.md`** (FLOW-15): no update is owed.
`docs/book/src/concepts/overview.md` holds pseudocode for the product's
components (orchestrator, session, drivers, executor, relay). Tier 1 is
contributor tooling and changes none of their flows, just as `tools/workflow`
has no entry there.

**Budget** (FLOW-13). The estimate is 6 runs and 30 steps. One run and 3
steps are spent, which leaves 27 steps.

| Steps left | Count |
|---|---|
| `p3-implement` claims (tasks 1 to 8 and 11 to 13) | 4 to 5 |
| `p3-export-problems`, `p3-front-matter-check` | 2 |
| `p4-review` (plus one claim if Review finds a fix) | 1 to 2 |
| `p5-guide`, `p6-publish`, `p6-merge-round` | 3 |
| Merge round, `m0` to `m8` (10 steps, two of them human) | 10 |
| **Total** | **20 to 22** |

The total fits within 27 steps, well below the runaway threshold of 60
(factor 2). Runs are the tight part. Six runs hold if implementation fits in
two runs and Jeff takes `p3-export-problems` before `p3-implement` finishes.
A late export, the serial merge queue, or the human waits in the merge round
(`m6-confirm-pr`, `m7-sign-off`) each add a run, so expect 6 to 8. Runs are
not part of the runaway check.

1. **Scaffold `tools/tier1`** (a1; LINT-05, UX-02, UX-04, GLOBAL-01,
   GLOBAL-03, TEST-01).
   - `go.mod` is module `github.com/buildwarden/buildwarden/tools/tier1`,
     with a `go` line no lower than `tools/workflow`'s. It requires
     `golang.org/x/tools` and `go.yaml.in/yaml/v3`, the same YAML library as
     `tools/workflow`. It also requires `tools/workflow`, with
     `replace github.com/buildwarden/buildwarden/tools/workflow => ../workflow`.
   - `cmd/tier1/main.go` parses the flags and subcommands in Design.
   - A `report` package holds the finding, hint and suppression types, the
     text output grouped by severity, the JSON output, `report.schema.json`,
     and exit codes 0, 1 and 2.
   - Tests, tier 0 (TEST-02): golden text and JSON for a fixed set of
     findings; the exit-code table (SHOULD and hints only give 0, MUST or
     MUST-review gives 1, usage errors give 2); every key the JSON emits is
     declared in `report.schema.json`.
2. **`diff` package** (a5; LINT-04, LINT-05, UX-04, GLOBAL-07, TEST-04).
   - Base: `-base REV`, or by default `git merge-base origin/main HEAD`. An
     unresolvable base exits 2 and prints the fix (`git fetch origin main`).
     It never falls back to a whole-repo run.
   - Changed paths come from `git diff --name-status -z` (renames
     included). Added and changed line ranges come from `--unified=0`. Paths
     stay `/`-separated as git prints them.
   - `Tier1-Allow: <RULE-ID> <path-glob>: <reason>` trailers are read from
     every commit in `base..HEAD`. A trailer with no reason is itself a
     finding, and each applied suppression is recorded with its commit.
   - Tests, tier 0 (TEST-02): table tests over a scratch git repository, as
     in `tools/workflow/check/check_test.go`. They cover added, modified,
     renamed and deleted paths, line ranges, valid trailers, a trailer with
     no reason, a trailer outside the range, and the unresolvable-base error.
     Commit dates and identities are fixed through the environment (TEST-04).
3. **`rules` package and the `rule-index` self-check** (a7; LINT-01,
   LINT-02, LINT-06, LINT-08).
   - The parser reads every `### <ID> — <title>` header in
     `docs/development/*.md`, with its `**Severity:**` and `**Check:**`
     lines. It skips `docs/development/README.md`, whose `RELAY-01` header is
     a format example. It accepts the qualified check forms in use, for
     example `automated (Arm A) + agent-judgment (Arm B)`.
   - `tier1 rules [-json]` prints the index.
   - `tools/tier1/rules.yaml` maps all 54 `automated`-tagged rules on `main`
     today to a check ID, an owner or `follow-up`, following the three
     Design tables. It also holds the path data for tasks 5 and 6: RELAY-05's
     schema/spec pairs, BOOT-07's bootstrap surface, and GLOBAL-08's seam to
     contract-document map.
   - The self-check runs as Design says: when `docs/development/` or
     `rules.yaml` is in the diff, and with `-all`.
   - Tests, tier 0 (TEST-02): the real `docs/development/` parses with no
     findings and every `automated` rule is mapped. Fixtures cover a header
     with no `Severity`, an invalid `Check`, an unmapped `automated` rule and
     a mapping to a removed rule.
4. **Design union and the FLOW-01 half of `design-front-matter`** (FLOW-01,
   FLOW-02, ORCH-01).
   - In a `coupling` package, parse every `docs/design/*.md` the diff adds
     or modifies, except `TEMPLATE.md`, at `HEAD` with the exported
     `check.ParseDesign`. Take the union of their `drivers`, `seams` and
     `rules` (Decision 6). Use `-design PATH` to name designs explicitly.
   - Changed code with no design in the diff is a FLOW-01 finding, cleared
     by `-no-design` or a `Tier1-Allow: FLOW-01` trailer. "Code" starts as
     every path outside `docs/` and top-level `*.md`; task 11 retunes it on
     history.
   - Tests, tier 0 (TEST-02): scratch-repo tables for one design, two
     designs (the release diff), `TEMPLATE.md` ignored, code with no design,
     docs-only with no design, and `-no-design`.
5. **Coupling checks** (a6; ORCH-01, FLOW-02, BOOT-07, RELAY-05, GLOBAL-08,
   EXEC-04, GLOBAL-04, TEST-02).
   - `drivers-claim`: compare the paths the diff reaches against the union's
     `drivers`. A shared package (`driver/*.go`, `relay/`, `ledger/`,
     `collector/`, `relaycfg/`, `bootstrap/`) needs every driver declared
     (the TEST-02 caveat).
   - `bootstrap-surface`: use BOOT-07's surface list from `rules.yaml`. The
     "relay's guest-facing endpoints" become named `relay/` files there.
     Check both directions of BOOT-07's automated part.
   - `schema-doc`: check both directions of RELAY-05's pairs.
   - `seam-contract`: check only the seams that have a contract document in
     `rules.yaml`. The others are `follow-up`.
   - The `tests-with-change` hint (GLOBAL-04) never fails and lists under
     `hints`.
   - Tests, tier 0 (TEST-02): a positive and a negative scratch-repo case
     for each check. That includes a shared-package change with too few
     drivers declared, and a `bootstrap.md` change with no design citing
     BOOT-07.
6. **`surface` extractor, `surface-docs` and `surface-removed`** (UX-01,
   ORCH-03, UX-03).
   - Extract cobra flag names (literal names passed to `Flags()` and
     `PersistentFlags()` definers) and `toml:` tags from the config types
     under `cmd/warden/`, using `go/parser` over each file's text at base
     (`git show <base>:<path>`) and at `HEAD`. No type-checking and no
     worktree are needed.
   - `surface-docs` fails when the set changed but neither
     `docs/book/src/guide/cli.md` nor `configuration.md` did.
   - `surface-removed` fails when a name present at base is gone at head,
     with no alias and no `Deprecated` / `MarkDeprecated` marking.
   - Tests, tier 0 (TEST-02): a fixture cobra command tree and config struct
     under `testdata/` cover an added flag, a removed flag, a deprecated
     flag, a renamed `toml` key, and no change.
7. **Analyzers** (RELAY-01, RELAY-07, EXEC-01, EXEC-05).
   - Each is a `go/analysis` pass in `analyzers/`, run through
     `go/analysis/checker` over the packages holding changed `.go` files,
     for the host `GOOS`. Findings are kept only on changed lines.
   - `ledger-single-writer` (RELAY-01) flags `l.writer.Write`, calls to
     `(*Ledger).sign`, and assignments to `prevSigRaw` outside
     `(*Ledger).loop`. It must also allow `(*Ledger).writeHeader`, which
     `NewLedger` calls before `go l.loop()` starts (`relay/ledger.go`). That
     allowance is a precision the Design row does not state.
   - `relay-diskless` (RELAY-07) flags `os.Create`, `os.WriteFile`,
     `os.OpenFile`, `os.CreateTemp`, `os.Mkdir*` and the `ioutil` writers in
     non-test `relay/` files. There are none on `main` today.
   - `executor-no-cgo` (EXEC-01) flags `import "C"` under `cmd/warden-io/`.
   - `large-literal` (EXEC-05) flags string literals over 4 KiB. It is
     SHOULD, so its findings are informational.
   - Tests, tier 0 (TEST-02): `analysistest` testdata with positive and
     negative cases for each analyzer, including `writeHeader` allowed and a
     new helper that writes the ledger flagged.
8. **The tier-1 golangci-lint config** (GLOBAL-01, GLOBAL-02, GLOBAL-03,
   UX-04).
   - `tools/tier1/golangci.yml` turns on `revive` `exported` (stutter check
     off), the `gofmt -s` formatter, `errorlint` errorf, and `forbidigo` for
     `panic` with `cmd/**` main packages and `_test.go` excluded.
   - `tier1` runs it from the repo root with
     `--new-from-merge-base=<base>` and JSON output, and maps each issue to
     its rule. If `golangci-lint` is missing, it exits 2 with the install
     command.
   - Tests, tier 0 (TEST-02): canned golangci JSON maps to the right rules
     and severities. TEST-01 forbids external tools in unit tests, so the
     live run is exercised through `make lint` (task 12).
9. **`p3-export-problems`, human, attended only** (a3; FLOW-14, FLOW-15).
   This is rework of existing code.
   - In `tools/workflow/check/check.go`, rename `(*Front).problems` to
     `Problems`, give it a doc comment, and update its one caller in
     `(*checker).design`.
   - Run `cd tools/workflow && go test ./... && golangci-lint run -c
     ../../.golangci.yml`. Commit on `feat/tier1-static-checks`, then run
     `wf done` for the step.
   - Unattended runs never do this task. If Jeff declines a3, he marks the
     step done with the decision in its note and records the answer on a3,
     and task 10 takes the fallback.
10. **`p3-front-matter-check`: the FLOW-02 half of `design-front-matter`,
    and `tier1 designs [-all]`** (a2, a3; FLOW-02, FLOW-12, FLOW-15).
    - Every touched design's front-matter goes through `check.Front.Problems`,
      with the feature name taken from the file stem.
    - `tier1 designs -all` validates every design that has front-matter. It
      lists a design with none as pre-standard (FLOW-12 allows that) without
      failing. Six designs on `main` have none until
      `design-front-matter-retrofit` merges, and a failing `-all` would block
      every release before then.
    - If a3 falls back, duplicate the key and enum rules instead. The parity
      test then runs the exported `check.Phase` (phase 1) over the same
      scratch-repo fixtures and compares its `front-matter (FLOW-02)`
      results with tier 1's.
    - Tests, tier 0 (TEST-02): valid and invalid fixtures for each FLOW-02
      key and enum, a missing key, and a pre-standard design under `-all`.
    - Run `make lint` and `make test` before `wf done`.
11. **Run against history** before task 12 blocks anything (a6, a7;
    LINT-07, TEST-04).
    - Run `tier1 -base <parent>` on the merge commits of PRs #60 to #64, and
      on each in-flight branch against its merge base: `container-inputs`,
      `design-front-matter-retrofit`, `firecracker-driver`,
      `native-loop-followups` and `workflow-bootstrap-archive`.
    - Record each finding and false positive in a "Notes" paragraph at the
      end of this section.
    - Retune any check that is noisy on known-good history, or demote it to
      a hint (the GLOBAL-04 treatment). A demotion changes a severity Design
      chose, so it is flagged for Review, and any rule sharpening is proposed
      for the PR description (FLOW-14).
    - If task 10 lands later, it reruns this over the same diffs.
12. **Wiring** (a4; LINT-03, TEST-01, TEST-02, GLOBAL-07, FLOW-03). This is
    rework of existing code.
    - `Makefile` `lint`: build `dist/tier1` from `tools/tier1`, run it, and
      run `golangci-lint` over `tools/tier1` with the root config.
      `Makefile` `test`: add `cd tools/tier1 && go test ./...`.
    - `.github/workflows/main.yml`:
      - A `tier1` job with `fetch-depth: 0`. It runs `tier1 -json` against
        the PR base SHA, or `github.event.before` on a push to `main`, and
        uploads the JSON.
      - A `tools/tier1` test job on `ubuntu-latest`, `macos-latest` and
        `windows-latest` (GLOBAL-07).
      - `tools/tier1` added to the `lint` and `tidy` jobs.
    - `.github/workflows/release.yml`: a `tier1` job that runs `tier1`
      against the previous `v*` tag, plus `tier1 designs -all`. The
      `release` job `needs` it.
    - Acceptance: `make lint` on this branch runs tier 1 over this feature's
      own diff and passes.
13. **Docs** (UX-01, FLOW-14, FLOW-15). This is rework of existing docs.
    - `CONTRIBUTING.md`: `make lint` now runs tier 1. Cover the base and how
      to fetch it, `GOOS=<os> make lint` for other platforms, the
      `Tier1-Allow` trailer, the exit codes, and `tier1 rules`.
    - `docs/book/src/contributing/structure.md`: a `tools/tier1/` entry next
      to `tools/workflow/`.
    - No edit to `docs/development/`, `.claude/` or the README's Deep-dive #2
      entry. Those changes, and any rule sharpening from task 11, go in the
      PR description for Jeff (FLOW-14, FLOW-08).

**Notes (task 11, run against history).** `tier1 -base <first parent>` ran
on the merge commits of PRs #60 to #64, and `tier1` (base `origin/main`) on
`container-inputs`, `design-front-matter-retrofit`, `firecracker-driver`,
`native-loop-followups`, `workflow-bootstrap-archive` and `main`, each in a
detached worktree with this branch's binary. Task 10 is not built yet, so the
FLOW-02 half of `design-front-matter` did not run; task 10 reruns these diffs.

- **Retuned: Go changes that touch only comments or layout no longer reach a
  path** for `drivers-claim`, `bootstrap-surface` and `seam-contract` (a
  token comparison with comments dropped). PR #62 corrected a comment in
  `driver/fixable.go`; with the design it edited incidentally as the claim,
  `drivers-claim` failed it for not declaring every driver.
- **Demoted to a hint, for Review:** `bootstrap-surface`'s "surface changed,
  `bootstrap.md` did not" direction. It fired on `container-inputs`
  (`driver/container/container.go`), whose change is not a model change, and
  BOOT-07 leaves that judgment to its agent part. The other direction,
  `bootstrap.md` changed with no design citing BOOT-07, stays MUST-review.
- **Demoted to a hint, for Review:** `schema-doc`'s "spec changed, no schema
  did" direction. PR #62 fixed prose in `ledger-spec.md`, which also
  describes the binary format and has no schema counterpart. A schema change
  without the spec still fails. A rule sharpening for the PR description:
  RELAY-05's "fails if one changes without the other" holds for a
  `metadata-<name>/` pair once `specs-migration` lands, and the reverse
  direction can be a MUST then.
- **True positives, kept:** PR #60 removed `--accept-unpinned` with no alias
  or deprecation (UX-03), and added the exported `BuildGuestCIDR` with no doc
  comment (GLOBAL-01). Its `gofmt` finding is SHOULD.
- **Calibration artifacts, no change:** on PRs #60 and #61, `rule-index`
  reported FLOW-16, FLOW-18 and UX-09 as mapped but missing, because the
  binary embeds this branch's `rules.yaml` and those rules came later. `make
  lint` builds tier 1 from the tree it checks, so the two always match.
- **Result:** after the retune, every in-flight branch and `main` exit 0.
  `container-inputs` carries one BOOT-07 hint. PRs #61 to #64 exit 0 apart
  from the PR #61 artifact; PR #60 exits 1 on its two true positives.
  GLOBAL-04 hints appear on most code diffs, as expected for a hint.

**Notes (task 10, the FLOW-02 half).** Built on `check.Front.Problems` (a3),
with the feature name taken from the file stem. `tier1 designs` reran over the
same diffs.

- **Pre-standard, two forms (FLOW-12).** A design with no front-matter is
  listed, not failed, under `-all`; in a diff it is listed only when it also
  had none at base, so a new design without front-matter fails. Archived
  designs kept as merged although they lack a later key are listed in
  `rules.yaml` `design_tolerate` by path and key, and their problems become
  hints: the three that lack `alters_relationships`
  (`design-front-matter-retrofit` a3), and `firecracker-driver-plan.md`,
  whose retrofit `feature` is the queue slug `firecracker-driver`, not its
  stem (that feature's a6). The fourth entry is dormant until the retrofit
  merges; flagged for Review, since it widens the hand-off by one design.
- **Result.** `tier1 designs -all` exits 0 on `main` (6 pre-standard, 3
  tolerated) and on the retrofit branch (0 pre-standard, 4 tolerated). PRs
  #60 and #61 touched the three archived designs and pass only through
  `design_tolerate`; #62 to #64, `container-inputs`, `native-loop-followups`,
  `workflow-bootstrap-archive` and `design-front-matter-retrofit` pass.
  `firecracker-driver` fails: it adds `firecracker-spike-plan.md` with no
  front-matter, a true positive for that branch's consolidation.

## Design review

- **r1, 2026-10-31: approved** (Jeff, attended session, FLOW-19), approved at
  `004d833`. Held after the design predated FLOW-19 and `wf migrate` put a review
  ahead of Implement. a1 to a7 were already confirmed.
  - Confirmed: the two demotions to hints from the history run (BOOT-07's
    "surface changed, `bootstrap.md` did not"; RELAY-05's "spec changed, no
    schema did").
  - Merging today's `main` (workflow-streamline) left tier 1 failing its own
    diff, so Implement continues with: map FLOW-19 in `rules.yaml` to the
    owner `wf` (A), and carry `design_review` in this design (B, done with
    this entry).
  - `tier1 designs -all` failed nine archived designs for lacking
    `design_review` (C). The fix belongs to the shared validator, not to tier
    1: buildwarden/buildwarden#76 exempts archived designs.
  - Keep the attended export of `Front.Problems` in `tools/workflow/check`
    (a3), made before FLOW-14 kept feature branches out of `tools/workflow/`,
    and record it in Consolidation. Record the plan's references to the old
    rounds and steps in Consolidation rather than rewriting the plan.

## Consolidation

`main` merged at `154833b` in `4b5266d` (#75, #76) and at `867219f` in
`539fec0` (#77), without conflicts; the fork point was #64 (`137feae`).

- **Landed since the fork:** #65 native-loop-followups, #67
  relay-gzip-framing, #69 native-leftovers, #66 workflow-bootstrap-archive,
  #70 and #71 relay-leaf-crl, #72 container-inputs, #73 and #74, #75
  workflow-streamline, #76 design-review-archived and #77
  design-front-matter-retrofit. #65 to #74 change no rule header or `Check:`
  line, so the rule index and `rules.yaml` need nothing from them; #65's
  `integration.yml` is the TEST-02 tier, not this one. The overlaps:
  - **#75 workflow-streamline** (one round). It added FLOW-19, which
    `rules.yaml` now maps to the owner `wf` (design review r1, A). The
    template dropped `round` and gained `design_review`, so the coupling
    fixtures carry `design_review` instead of `round` (`79af3f5`) and this
    design drops `round: parallel`. LINT-03's per-PR sign-off is now the
    human's merge, with the MUST findings listed in the PR body; Decision 5's
    "human sign-off" reads as that merge, and the printed suppressions are
    what it sees.
  - **#76** exempts archived designs from `design_review` in the shared
    validator. Its new test called the unexported `problems`; the merge made
    it call the exported `Problems` (`79af3f5`). That and the a3 export
    (`47f80c3`, Jeff, attended) are this branch's only `tools/workflow` edits.
    The export was made before FLOW-14 kept feature branches out of
    `tools/workflow/`, and r1 keeps it.
  - **#77** gave the six pre-standard designs front-matter. `tier1 designs
    -all` on the merged tree: 17 checked, 2 failing (`native-host-loop.md`,
    `native-loop-followups.md`, missing `design_review`; archived by
    `pre-release-cleanup`), 0 pre-standard, 4 tolerated. All four
    `design_tolerate` entries still match a problem, so none is stale: #76
    exempts only `design_review`, not the three designs' missing
    `alters_relationships`, and the `firecracker-driver-plan.md` entry, dormant
    when the plan was written, is now the one that keeps it passing.
- **Terminology:** "merge round" in Prior art, a4, the Mapped-elsewhere table,
  the task-10 notes and `rules.yaml`'s FLOW-15 owner now read "Publish
  consolidation". The Implementation plan's `p3-*` steps, `p6-merge-round`, the
  `m0` to `m8` merge round and its budget table predate the one-round loop and
  are kept as written (r1): they ran as `f3-implement` claims plus the attended
  export, `f4-review` and Publish. "Tier 1" is overloaded: LINT-02's static
  checks here, TEST-02's integration tier in `integration.yml` and
  `testing.md`. The design and `CONTRIBUTING.md` say "test tier" for the
  latter; a qualifier in the standards is proposed below.
- **Abstractions:** one front-matter validator. `coupling/frontmatter.go` and
  `coupling.go` call `check.ParseDesign` and `check.Front.Problems` (a3), so
  tier 1 and `wf` cannot disagree on FLOW-02; no key or enum list is
  duplicated in `tools/tier1`. Base resolution, line ranges and trailers live
  only in `diff/`; the surface extractor is shared by `surface-docs` and
  `surface-removed`.
- **Seams:** none declared; no versioned seam changed on `main` overlaps.
- **FLOW-15 sweep:** `make deadcode` (linux, darwin, windows; run with a
  go1.26.3 build of `deadcode` v0.38.0, since `dist/deadcode` was a go1.24
  build) reports nothing from this feature, which changes no product code, and
  `deadcode -test ./...` in `tools/tier1` and `tools/workflow` reports nothing.
  Unreachable on all three, as on `main`: `Collector.BytesWritten`,
  `Relay.CAFingerprint` and `Relay.Ledger` (exported library API), and the
  `driver/vz` helpers (cgo-only, checked on macOS). Duplication: none, as
  above. `overview.md`: no update owed, as the plan says.
- **Proposed, not made here (FLOW-14):**
  - queue the follow-up item for the `follow-up` rules in `rules.yaml` (a7),
    with the rule sharpenings below;
  - RELAY-05: the reverse direction (spec without schema) can become MUST once
    `specs-migration` pairs each `metadata-<name>/` schema with its document;
  - TEST-02 and LINT-02: name the tiers "test tier" and "lint tier" where
    both could be meant;
  - `docs/development/README.md`: Deep-dive #2 is done once this merges and
    its entry can go, pointing to this design.

## Rollback notes

None. Nothing validated was reverted; the two checks demoted to hints by the
history run are recorded in the task-11 notes and the Archive summary.

## Archive summary

Archived at Publish (`f6-open-pr`) on 2026-10-31, after one design round,
design review r1 (Jeff), one fix round from tier-2 review, and merges of
`main` at `154833b` and `867219f`.

- **Built.** `tools/tier1`, its own Go module, run by `make lint` (built for
  the host, run with the caller's `GOOS`), a `tier1` CI job on PRs and pushes
  to `main`, a three-OS `tier1-test` job, and a release job that runs it
  against the previous `v*` tag plus `tier1 designs -all` and that `release`
  needs. Checks: `rule-index`; `design-front-matter` (FLOW-01 and FLOW-02 on
  the shared `wf` validator); `drivers-claim`, `bootstrap-surface`,
  `schema-doc`, `seam-contract`; `surface-docs` and `surface-removed`;
  analyzers `ledger-single-writer`, `relay-diskless`, `executor-no-cgo`,
  `large-literal`; a diff-scoped golangci config for GLOBAL-01 to 03; the
  GLOBAL-04 hint. `rules.yaml` maps all 55 `automated` rules: 21 to checks,
  21 to owners, 13 `follow-up`. `tier1 rules -json` is the index tier 2 gets.
- **Decisions taken:** its own module (a1); three mechanisms, no ripgrep;
  diff-scoped with no whole-repo fallback, PR base or `github.event.before`
  in CI; severity decides the exit code (0, 1, 2); `Tier1-Allow:` commit
  trailers with a required reason, printed (a5); the designs in the diff are
  the claim; the attended export of `Front.Problems` (a3); `designs -all` at
  release only (a2); wiring inside `make lint` (a4); the first slice (a7).
  A push to `main` passes `-no-design`, since FLOW-01 is judged on the PR and
  the `no-design` label (Review MUST 2). Hints, never failing: GLOBAL-04 (a6),
  and two demotions from the history run that r1 confirmed: BOOT-07's
  "surface changed, `bootstrap.md` did not" and RELAY-05's "spec changed, no
  schema did".
- **Rejected:** a package in `tools/workflow` or the product module; a
  golangci plugin; ripgrep scripts; a suppressions file or inline comments;
  an archive-wide check per PR; a `wf`-run `make tier1`; duplicating the
  validator; for the release gate (Review MUST 3), starting the base at tier
  1's first commit (Jeff, option a). The base stays the previous tag, and
  `pre-release-cleanup` clears the 25 release-range code findings and
  archives the two native designs before the next tag.
- **Validated.** Tier 0 tests in every package; the run over PRs #60 to #64
  and five in-flight branches (one retune, two demotions, true positives on
  #60 and `firecracker-driver`); tier-2 reports `79af3f5cf53d` (3 MUST, 6
  SHOULD) and `5460fee0e61e` (MUST resolved).
- **For future designs touching these `concepts`:** a new `automated` rule
  needs a `rules.yaml` entry or tier 1 fails the change; path data (drivers,
  bootstrap surface, schema pairs, seams, surface) lives there, so
  `specs-migration` and new seams edit data, not code. `seams` is empty, so
  `seam-contract` checks nothing until a seam gets a contract document. The
  release gate fails until `pre-release-cleanup` lands. `ci-path-filters` must
  keep the `tier1` job for `docs/design/**`. Known gaps, the Review's SHOULD
  findings: `Tier1-Allow` matched on any body line, not trailers only;
  suppressions do not list what they cleared; analyzers and golangci read the
  working tree while ranges come from HEAD; directive comments
  (`//go:build`) dropped by the comment-only retune; a bad `-design PATH`
  ignored silently; three golangci-lint versions in use.
