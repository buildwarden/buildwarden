---
feature: ci-path-filters
status: active
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [TEST-01, TEST-02, LINT-02, LINT-03, LINT-04, FLOW-01, FLOW-11, FLOW-14, FLOW-17, FLOW-18, GLOBAL-07, UX-01]
concepts: [ci, path-filters, github-actions, required-checks, docs-only, tier0, tier1, mdbook]
related: [tier1-static-checks.md]
---

# CI runs only the checks a change's paths need

## Problem

Every pull request runs the whole `ci` workflow (`.github/workflows/main.yml`):
`lint`, `tier1`, `tier1-test` on three operating systems, `test`, `tidy`, and
`build` on two. A docs-only change, such as a guide or README fix, a design
landed on its own, or a design-archive correction after a merge, cannot change
the outcome of Go lint, Go tests, `go mod tidy` or a build, yet it waits for
all of them and spends macOS and Windows runner minutes (Jeff, 2026-10-30:
"docs-only updates should skip the testing rounds since there are no code
changes"). Pushes to feature branches do not run `ci` today, so the saving is
on PRs; a feature PR that carries code still runs everything.

The reverse problem exists too: a PR that edits `docs/book/` gets no mdbook
build at all, because `docs.yml` builds only on a push to `main`. A broken
`SUMMARY.md` is found after the merge, when the Pages deploy fails.

## Prior art

- **`tier1-static-checks.md`** (archived). Its Decision 8 and task 12 added
  the `tier1`, `tier1-test` jobs and the `tools/tier1` steps in `lint` and
  `tidy`. Its archive summary says: "`ci-path-filters` must keep the `tier1`
  job for `docs/design/**`". This design keeps more than that: `tier1` runs on
  every PR (Decision 3). Its `rules.yaml` `not_code` list (`docs/`, `*.md`,
  `.github/`, `LICENSE`) answers a different question, "does this need a
  design" (FLOW-01), so it is not reused as the CI classification; see
  Decision 2.
- **`integration.yml`**. Already path-filtered at the workflow level on the
  TEST-02 tier-1 paths. It is not a required check, so a skipped run has no
  cost. Kept as is (Decision 6).
- **TEST-02**. "Per-PR pays for the fast gate plus the tiers the change
  actually touches", with the caveat that scoping must be conservative. This
  design applies the same idea to tier 0 for paths that cannot reach Go code.
- **TEST-01**. "`make test` ... is the gate every change passes." Read
  literally, a docs-only PR that skips `test` breaks it. This design proposes
  rewording TEST-01 and TEST-02 (Proposed rule changes); it does not edit
  `docs/development/` (FLOW-14).
- **FLOW-18** (`flow18.yml`) and **CodeQL default setup**. Both stay
  unfiltered on every PR.
- No other design in `docs/design/` names `ci`, `github-actions` or
  path filtering in its front-matter or body.

## Decisions

1. **A `changes` job plus per-job `if:`, and a `ci-ok` gate job.** (a1)
   - `main.yml` gains a first job, `changes`, that classifies the PR's
     changed paths and exposes boolean outputs. Each filtered job `needs:
     changes` and has an `if:` on those outputs. A job whose `if:` is false
     reports "Skipped", which GitHub treats as passing for a required check.
   - A final `ci-ok` job `needs` every other job in `main.yml`, runs with
     `if: always()`, and fails when any of them failed or was cancelled
     (including `changes` itself). Success and skipped both pass.
   - Rejected: workflow-level `on.pull_request.paths-ignore`. A workflow that
     does not run reports no status at all, so a required check on it stays
     "Expected" forever. It also cannot express "run `tier1` but not `test`"
     inside one workflow.
   - Rejected: required checks on the individual jobs. A skipped matrix job
     reports under its bare name (`build`), not its matrix names
     (`build (ubuntu-latest)`), so requiring the matrix names would leave a
     docs-only PR pending. One gate job also means the list of required checks
     does not change when jobs are added.
   - `main` has no branch protection today (checked 2026-10-30 at intake).
     This feature does not turn any on. `ci-ok` is the check to require when
     Jeff does (LINT-03's "required PR check a human toggles").
2. **Conservative path classes.** (a2) A changed path belongs to the first
   class that matches:

   | Class | Paths | Why |
   |---|---|---|
   | `rules` | `docs/development/**` | `TestRealIndex` (`tools/tier1/rules`) parses the real rule files, so `test` and `tier1-test` must run. |
   | `book` | `docs/book/**` | Docs, and also input to the mdbook build (Decision 7). |
   | `docs` | the rest of `docs/**`; `*.md` at the repository root (`README.md`, `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `SECURITY.md`) | No Go package lives under `docs/`, `//go:embed` cannot reach a parent directory, and no test reads these files. |
   | `code` | everything else, including `.github/**`, `.claude/**`, `tools/**`, `schemas/**`, `go.mod`, `go.sum`, `Makefile`, `.golangci.yml`, `LICENSE`, and `*.md` below the root | Default. Anything not positively known to be docs runs the full set. |

   The outputs are `code` (any `code` path), `gotests` (`code` or `rules`),
   and `book` (any `book` path). A PR with no changed paths, or one the job
   cannot classify, gets every output `true`.
   - Rejected: reusing tier 1's `not_code`. It treats `.github/` and nested
     `*.md` as not code, which is right for FLOW-01 (no design needed) and
     wrong here (a workflow edit must run the workflow).
   - Rejected for now: narrower classes such as `tools/tier1/**` only running
     `tier1-test`. Possible later, once this has run for a while; the win is
     small next to docs-only.
3. **What runs when.**

   | Job | Runs on a PR when | Notes |
   |---|---|---|
   | `changes` | always | |
   | `tier1` | always | Diff-scoped already (LINT-04). It is the job that validates design front-matter, runs the rule self-check when `docs/development/` changes, and the UX-01 and RELAY-05 coupling checks, all of which docs-only PRs need. |
   | `lint` | `code` | |
   | `tidy` | `code` | |
   | `build` (2 OS) | `code` | |
   | `test` | `gotests` | |
   | `tier1-test` (3 OS) | `gotests` | |
   | `book` (new) | `book` | Decision 7. |
   | `ci-ok` | always | Decision 1. |
   | `flow18.yml` `sessions-private` | always | Separate workflow, not filtered (FLOW-18). |
   | CodeQL default setup | always | Repository setting, not a workflow file; nothing to change. |
   | `integration.yml` | its existing `paths` | Decision 6. |

   (a3: `tier1` on every PR rather than only on `docs/design/**`.)
4. **Pushes to `main`, tags and every non-PR event run the full set.** (a5)
   The `changes` job sets every output `true` unless the event is
   `pull_request`. `main.yml` triggers only on push to `main` and PRs today,
   so in practice this is the push to `main`, which is what keeps `main`'s
   history fully checked (TEST-02's backstop alongside the release gate).
   `release.yml`, `relay-image.yml`, `auto-tidy.yml` and `docs.yml` are not
   touched.
5. **The classification is a short shell step in `main.yml`, fail-open.** (a4)
   - On a `pull_request` event the checkout is GitHub's merge commit, so
     `git diff --name-only --no-renames HEAD^1 HEAD` (with `fetch-depth: 2`)
     is exactly what the PR changes against the current base. `--no-renames`
     lists both sides of a rename, so moving a Go file into `docs/` still
     counts as a `code` change.
   - A `case` statement over each path sets the three outputs. If `git diff`
     fails or prints nothing, every output is `true`. The step never emits
     `false` unless it positively classified every path.
   - Rejected: `dorny/paths-filter`. It works, but it is a third-party action
     in the PR path for what is ten lines of shell, and its rename handling
     and negation semantics need their own reading. The repository's only
     third-party actions today are in scheduled and release workflows.
   - Rejected: classifying inside `tools/tier1`. It would couple CI gating to
     building tier 1 first, which needs Go and costs more than the jobs it
     saves on a docs-only PR.
6. **`integration.yml` keeps its own filter.** (a6) Its workflow-level
   `paths` already match TEST-02's tier-1 list, and it is not a required
   check, so the "no status" problem in Decision 1 does not apply. Folding it
   into `main.yml` would turn a 45-minute job into part of every code PR's
   wait. Unchanged.
7. **Docs PRs that touch `docs/book/**` gain an mdbook build.** (a7)
   - A `book` job in `main.yml` installs the same pinned mdBook as
     `docs.yml` (v0.4.44) and runs `mdbook build docs/book`. Build only: no
     Pages artifact, no deploy, `contents: read`.
   - Rejected: a `pull_request` trigger on `docs.yml`. That workflow holds
     `pages: write` and `id-token: write`, and its `concurrency: pages` group
     with `cancel-in-progress: true` would let a PR run cancel a real deploy.
   - On a push to `main` the `book` job also runs (Decision 4), duplicating
     `docs.yml`'s build for about 20 seconds. Accepted for the simplicity of
     "non-PR runs everything".

## Questions

### Blocking

None.

### Assumed

Each is "proceeding with the recommendation; confirm" (FLOW-07, FLOW-19).

- **a1. Mechanism.** Options: (A) `changes` job, per-job `if:`, `ci-ok` gate
  job; (B) workflow-level `paths-ignore` on `main.yml`; (C) a separate
  docs workflow plus `paths-ignore`. **Recommended: A** (Decision 1). This
  feature does not configure branch protection; `ci-ok` is what to require
  when you do.
- **a2. Path classes.** Options: (A) docs-only is `docs/**` and root `*.md`,
  with `docs/development/**` still running `test` and `tier1-test`;
  (B) the same but `docs/development/**` fully docs-only, relying on
  `tier1`'s self-check alone; (C) tier 1's `not_code` list. **Recommended:
  A** (Decision 2): `TestRealIndex` reads those files, and a rules change is
  rare enough that the extra minutes do not matter.
- **a3. `tier1` scope.** Options: (A) every PR; (B) only PRs touching
  `docs/design/**` or `code`, as the tier1 design's note literally says.
  **Recommended: A** (Decision 3): tier 1 also carries the rule self-check
  and the docs coupling checks, and it is diff-scoped, so it is cheap.
- **a4. Classifier.** Options: (A) inline shell over `git diff HEAD^1 HEAD`
  on the merge commit, fail-open; (B) `dorny/paths-filter` pinned to a
  commit; (C) a `tools/tier1` subcommand. **Recommended: A** (Decision 5).
- **a5. Non-PR events.** Options: (A) push to `main` and any other non-PR
  event runs everything; (B) filter pushes against `github.event.before`
  too. **Recommended: A** (Decision 4).
- **a6. `integration.yml`.** Options: (A) unchanged; (B) move its filter into
  the `changes` job and make it a `main.yml` job. **Recommended: A**
  (Decision 6).
- **a7. mdbook on PRs.** Options: (A) a build-only `book` job in `main.yml`
  for PRs touching `docs/book/**`; (B) a `pull_request` trigger on
  `docs.yml`; (C) none. **Recommended: A** (Decision 7).
- **a8. Rule wording.** TEST-01 says `make test` is "the gate every change
  passes", and TEST-02 says tier 0 runs "on every push to `main` and every
  PR". Options: (A) propose the rewording below in the PR description for a
  process item, and land this feature against the current text, reading
  "every change" as every change that can affect the gate's outcome; (B) hold
  this feature until a process item rewords the rules first. **Recommended:
  A.** Either way the change to `docs/development/` is yours (FLOW-14).

## Design

### `main.yml` after the change

```yaml
jobs:
  changes:
    runs-on: ubuntu-latest
    outputs:
      code: ${{ steps.classify.outputs.code }}
      gotests: ${{ steps.classify.outputs.gotests }}
      book: ${{ steps.classify.outputs.book }}
    steps:
      - uses: actions/checkout@v4
        with: {fetch-depth: 2, persist-credentials: false}
      - id: classify
        env: {EVENT: '${{ github.event_name }}'}
        run: |   # sketch; the shape of the step, not its final text
          all() { printf 'code=true\ngotests=true\nbook=true\n' >> "$GITHUB_OUTPUT"; }
          [ "$EVENT" = pull_request ] || { all; exit 0; }
          files=$(git diff --name-only --no-renames HEAD^1 HEAD) && [ -n "$files" ] || { all; exit 0; }
          code=false rules=false book=false
          while IFS= read -r p; do
            case "$p" in
              docs/development/*) rules=true ;;
              docs/book/*) book=true ;;
              docs/*) ;;
              */*) code=true ;;
              *.md) ;;
              *) code=true ;;
            esac
          done <<< "$files"
          gotests=$code; [ "$rules" = true ] && gotests=true
          printf 'code=%s\ngotests=%s\nbook=%s\n' "$code" "$gotests" "$book" >> "$GITHUB_OUTPUT"

  lint:       {needs: changes, if: "needs.changes.outputs.code == 'true'", ...}
  tier1:      {needs: changes, ...}            # always
  tier1-test: {needs: changes, if: "needs.changes.outputs.gotests == 'true'", ...}
  test:       {needs: changes, if: "needs.changes.outputs.gotests == 'true'", ...}
  tidy:       {needs: changes, if: "needs.changes.outputs.code == 'true'", ...}
  build:      {needs: changes, if: "needs.changes.outputs.code == 'true'", ...}
  book:       {needs: changes, if: "needs.changes.outputs.book == 'true'", ...}

  ci-ok:
    needs: [changes, lint, tier1, tier1-test, test, tidy, build, book]
    if: always()
    runs-on: ubuntu-latest
    steps:
      - env: {NEEDS: '${{ toJSON(needs) }}'}
        run: jq -e 'all(.[]; .result == "success" or .result == "skipped")' <<< "$NEEDS"
```

`tier1` `needs: changes` only so that `ci-ok`'s view is uniform and a broken
`changes` job is visible once; it has no `if:`. A job skipped because a job it
`needs` failed reports `skipped`, which is why `ci-ok` also lists `changes`:
a failed classifier turns `ci-ok` red instead of quietly passing.

The `labeled` / `unlabeled` PR types stay: adding `no-design` reruns the
workflow, and on a docs-only PR that rerun is now cheap.

### Docs

- `CONTRIBUTING.md`, Pull requests step 4: CI runs lint, tests and builds on
  PRs that change code; a PR that changes only `docs/` (outside
  `docs/development/`) or top-level Markdown runs tier 1, FLOW-18 and, for
  `docs/book/`, an mdbook build. `ci-ok` sums up the run.
- `docs/book/src/contributing/structure.md` has no CI section, so no change.
- No `overview.md` change: no product component's flow changes (FLOW-15).

### Proposed rule changes (for a process item; not edited here, FLOW-14)

- TEST-01: "`make test` ... is the gate every change that can affect it
  passes; changes confined to documentation paths that no test reads skip
  it in CI, and every push to `main` runs it."
- TEST-02, "Where each tier runs today", tier 0: "the `ci` workflow, on every
  push to `main` and on every PR that changes a path outside the docs-only
  set (`docs/**` except `docs/development/**`, and top-level `*.md`)."

## Cross-driver impact

`drivers` is empty. The change is confined to `.github/workflows/main.yml` and
contributor docs. No driver tree, shared package or `driver.Driver` contract
is reached, and every PR that touches one still runs the full tier-0 set and
the existing `integration.yml` filter, so no driver loses coverage (ORCH-01,
TEST-02 caveat).

## Validation plan

- **Tier 0.** No Go code changes; `make test` and `make lint` stay green.
  The workflow YAML is parsed by GitHub on push.
- **This branch's own PR.** It touches `.github/`, so it is `code`: every
  job runs and `ci-ok` passes. That shows the gate passes when everything
  succeeds.
- **Classifier table check, before the PR opens.** The classify step's text
  is copied out of `main.yml` into a scratchpad script that takes the file
  list on stdin and is run against: root `README.md`; `driver/README.md`;
  `docs/design/x.md`; `docs/development/process.md`;
  `docs/book/src/SUMMARY.md`; `.github/workflows/main.yml`; `go.mod`; a
  `--no-renames` pair for `relay/x.go` moved to `docs/x.go`; a non-PR event;
  and an empty list. Expected outputs are the Decision 2 table. The result
  goes in the PR description.
- **Gate logic.** The `ci-ok` expression is checked in the same script
  against `needs` JSON with each documented result (`success`, `failure`,
  `cancelled`, `skipped`), including a failed `changes` with skipped
  dependents.
- **Docs-only behavior on GitHub.** It cannot be shown on this PR: the
  merge-commit diff of any branch built on this one includes `.github/`, and
  `main.yml` runs only for PRs to `main`. It is confirmed on the first
  docs-only PR after the merge, checking that `changes`, `tier1`, `ci-ok` and `flow18` ran, the Go
  jobs report Skipped, and `ci-ok` is green. If not, the fix is a revert of
  this one workflow file, recorded under Rollback notes.
- **Native validation.** None; nothing host-specific. `native_validation`
  and `tier2_gaps` are empty.

## Implementation plan

1. **`changes` job** in `.github/workflows/main.yml` (Decisions 2, 4, 5;
   TEST-02, LINT-04). Checkout with `fetch-depth: 2` and no persisted
   credentials; the classify step as sketched, fail-open.
2. **Gate existing jobs** (Decision 3; TEST-01, TEST-02, GLOBAL-07). Add
   `needs: changes` to `lint`, `tier1`, `tier1-test`, `test`, `tidy`,
   `build`, with the `if:` from the table; `tier1` gets no `if:`. Rework of
   tier1-static-checks' task 12 jobs; their steps are unchanged.
3. **`book` job** (Decision 7; UX-01). Build-only mdBook v0.4.44, same install
   lines as `docs.yml`, `permissions: contents: read`.
4. **`ci-ok` gate job** (Decision 1; LINT-03, FLOW-11). `needs` every job,
   `if: always()`, `jq` over `toJSON(needs)`.
5. **Docs** (UX-01). `CONTRIBUTING.md` Pull requests step 4 as above.
6. **Validation** as in the Validation plan; classifier table result and the
   docs-only run's link go in the PR description, with the Proposed rule
   changes for Jeff (FLOW-14, FLOW-08).

**Notes (phase 3).** The classify step and the `ci-ok` command were extracted
from `main.yml` with a YAML parser and run under `bash -e`, with `git` stubbed
to print each file list. Outputs as `code`/`gotests`/`book`: `README.md`
false/false/false; `driver/README.md` true/true/false; `docs/design/x.md`
false/false/false; `docs/development/process.md` false/true/false;
`docs/book/src/SUMMARY.md` false/false/true; `.github/workflows/main.yml`,
`go.mod` and the `--no-renames` pair `relay/x.go` + `docs/x.go` true/true/false;
a design plus a book file false/false/true; a non-PR event, an empty diff and a
failing `git diff` true/true/true. All match Decision 2; a real `git mv` of
`relay/x.go` to `docs/x.go` also lists both paths and classifies as code. The
`ci-ok` `jq` exits 0 for all-success and for skipped Go jobs, and non-zero for
a `failure`, a `cancelled`, a failed `changes` with skipped dependents, and
malformed input. actionlint 1.7.7 reports no findings on `main.yml`.


**Notes (review 458a36af2b41).** Applied in Review: Go source and module files
(`*.go`, `go.mod`, `go.sum`, `go.work`, at any depth) classify as code even under
`docs/` (MUST 1: the root module compiles them, so `docs/ex/bad.go` would have
skipped lint, test and build); `CONTRIBUTING.md` sets `book` (the book includes
it); `git -c core.quotePath=false` keeps non-ASCII paths matchable; the `book`
checkout sets `persist-credentials: false`. Re-checked: `docs/ex/bad.go` and
`docs/x/go.mod` give code/gotests true; `CONTRIBUTING.md` gives book only;
`docs/book/src/café.md` gives book; the earlier table's rows are unchanged.

## Design review

- **r1, 2026-10-31: approved** (Jeff, attended session, FLOW-19).
  Approved at `3d5d382`. a1 to a8 confirmed as recommended; for a8 the TEST-01 /
  TEST-02 rewording is queued as its own process item.

## Consolidation

## Rollback notes

## Archive summary
