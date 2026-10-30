---
feature: tier1-static-checks
status: draft
round: parallel
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
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
  `make deadcode` is the FLOW-15 sweep. It is whole-repo and run in the merge
  round, so it stays outside the diff-scoped tier 1 (LINT-04) and is not
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
    diffs violate a MUST check. Their merge-round consolidation (FLOW-10) is
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
| `make deadcode`, merge round | FLOW-15 (whole-repo by design, not diff-scoped) |
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

Planning (phase 2) expands this list into tasks. The order:

1. Scaffold `tools/tier1`: module, `cmd/tier1`, the `diff` package (base
   resolution, changed files and lines, `Tier1-Allow` trailers), text and
   JSON output, exit codes (LINT-04, LINT-05, UX-02, UX-04, TEST-01).
2. `rules`: the index parser, `tier1 rules -json`, `rules.yaml` covering every
   `automated` rule, and the self-check (LINT-01, LINT-02, LINT-06, LINT-08).
3. Coupling checks: `drivers-claim`, `bootstrap-surface`, `schema-doc`,
   `seam-contract`, the GLOBAL-04 hint, and the design union (ORCH-01,
   BOOT-07, RELAY-05, GLOBAL-08, EXEC-04, GLOBAL-04, FLOW-02).
4. `surface` extractor, then `surface-docs` and `surface-removed` (UX-01,
   ORCH-03, UX-03).
5. Analyzers: `ledger-single-writer`, `relay-diskless`, `executor-no-cgo`,
   `large-literal` (RELAY-01, RELAY-07, EXEC-01, EXEC-05).
6. `golangci.yml` for GLOBAL-01, GLOBAL-02 and GLOBAL-03, run by `tier1` with
   `--new-from-merge-base` (GLOBAL-01, GLOBAL-02, GLOBAL-03).
7. `p3-export-problems` (`human` affinity, a3). This is an attended edit
   exporting `check.Front.problems` from `tools/workflow`. Then build
   `design-front-matter` and `tier1 designs [-all]` on it (FLOW-02, FLOW-12,
   FLOW-14, FLOW-15). If a3 falls back, this task becomes the duplicate plus
   a parity test, and it takes `cloud` affinity.
8. Run against history and the in-flight branches. Retune or demote the noisy
   checks and record the results (LINT-07, TEST-04).
9. Wiring: `make lint` and `make test` cover `tools/tier1`; the `tier1` job
   in `main.yml`, with tests on three OS runners; the release job in
   `release.yml` (LINT-03, GLOBAL-07, FLOW-03).
10. Docs: `CONTRIBUTING.md` (running tier 1, suppressing a finding) and
    `contributing/structure.md` (`tools/tier1`) (UX-01, FLOW-14). The README's
    Deep-dive #2 entry and any rule sharpening found in step 8 are proposed
    to Jeff in the PR description, not edited (FLOW-14).

## Consolidation (merge round)

## Rollback notes

## Archive summary
