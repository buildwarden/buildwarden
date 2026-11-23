---
feature: sync-pr-tier1
status: draft
drivers: []              # CI configuration, rule text and the loop's sync check; no driver tree is reached
seams: []                # no GLOBAL-08 seam changes
surfaces: []             # CI and `wf` are the loop's tools, not a user-facing surface of the product
repos: []                # only buildwarden/buildwarden
merge_first: []
areas: [build, rules, book, loop]
                         # build: .github/workflows/main.yml's tier1 job; rules: FLOW-01's and
                         # FLOW-14's wording in docs/development/process.md; book: CONTRIBUTING.md's
                         # "No design" bullet; loop: tools/workflow/check/merge.go (SyncShape, shared
                         # with SyncHead), tools/workflow/cmd/wf/main.go (wf sync-shape, and
                         # wf sync-program -head's refusal) and .claude/agents/wf-publish.md's sync
                         # paragraph. Intake recorded tier1: tools/tier1/ is not edited (tier1's
                         # -no-design flag already exists), so it is not declared
security_boundary: none  # no product boundary moves; what CI judges on a loop-merged PR is the
                         # process's own gate, reviewed under FLOW-20's reserved rules
alters_relationships: false  # no product component's role or interaction changes (FLOW-02)
design_review: required  # FLOW-20's process test may not settle it: by effect it relaxes a check
                         # that stops the loop (tier1 on the sync PR, which the sync gate's point 3
                         # requires green) and takes over the no-design label Jeff applied by hand on
                         # #140, a gate of his; and it has rule changes (FLOW-14, FLOW-19). P2 fails
                         # closed
native_validation: []    # CI configuration and git-only Go; nothing runs on a native host
tier2_gaps: []
rules: [FLOW-01, FLOW-08, FLOW-14, FLOW-15, FLOW-17, FLOW-19, FLOW-20, LINT-02, LINT-04, LOOP-09,
        PROFILE-01, GLOBAL-09, GLOBAL-10, GLOBAL-11, UX-04]
rule_changes:
  - {rule: FLOW-01, change: amend, paths: [docs/development/process.md, .github/workflows/main.yml,
     tools/workflow/check/merge.go, tools/workflow/check/merge_test.go]}
                         # tools/workflow/cmd/wf/ and .claude/agents/wf-publish.md are machinery:
                         # they change through this process item, not a rule_changes path (FLOW-14)
  - {rule: FLOW-14, change: amend, paths: [docs/development/process.md, .github/workflows/main.yml]}
concepts: [ci, tier1, no-design, program-sync, program-trunk, enforcement, sync-gate, workflow]
related: [program-north-star.md, design-approved-rule-changes.md, tier1-static-checks.md,
          ci-path-filters.md]
---

# Tier 1 judges a program sync PR as its merge's push: FLOW-01 and the enforcement comparison are not run again on main's merged history

## Problem

`.github/workflows/main.yml`'s `tier1` job diffs a PR against its base and
runs FLOW-01's automated half (`NoDesignInDiff`: code changed and the diff
carries no design) unless the PR has the `no-design` label. A push is not
judged on FLOW-01: it spans merged PRs that each passed it, and its payload
has no labels (`FLAGS: ... github.event_name == 'push' ... && '-no-design'`,
since 5460fee).

A program sync PR (FLOW-20, program-north-star Decision 14) has head
`sync/program-<main sha>`, base `program/<name>`, and is a merge of `main`
into the program trunk with conflict-only fixes. Its diff against the
program trunk is every `main` commit the trunk does not have yet. Tier 1
judges that as the PR's own new work. When none of those commits carries a
design, FLOW-01 fails:

- **#140** (`sync/program-168cf9503d4b`, `main` into
  `program/componentization`) failed tier 1 with a FLOW-01 MUST on
  `driver/layercache_test.go`, which came from #139, a `no-design` PR that
  had passed. Its range was #138 and #139, both `no-design`. Jeff labelled
  #140 `no-design` by hand as the stopgap (2026-11-23).

The sync gate (`check.SyncReady`, point 3) needs every check on the head
green, so until someone labels the PR the sync cannot merge, and the
program trunk lags `main`.

The job's second step, **main's enforcement** (FLOW-14,
`tools/workflow/enforcement-check.sh`), has the same blind spot and no
label clears it. It runs on PRs only because "the approvals it reads name
commits on the PR's branch, which a squash merge leaves off main"
(design-approved-rule-changes, Decision 6 in CI). A sync carries `main`'s
squash commits, so whenever `main` landed a change under `tools/tier1/` or
`tools/workflow/check/` the step runs on the sync, builds the program
trunk's tier 1, and any finding an approved `main` rule change dropped
fails as unapproved: `approvedRules` loads each design at its
`approved at <sha>`, a commit of the deleted feature branch that the
sync's history does not have (`check/rulechange.go`, `cleared`). It has
not fired yet; 27 of `main`'s first-parent commits 2026-10-26..11-23 touch
those paths.

## Prior art

- **FLOW-01's push exemption** (5460fee, `main.yml`'s comment): a change
  made of PRs that each passed FLOW-01 is not judged again. This design
  extends the same reasoning to the PR whose merge is such a push, and
  writes both into FLOW-01 (the push exemption was never in the rule's
  text).
- **design-approved-rule-changes, Decision 6 in CI:** the enforcement
  comparison runs on PRs only, because squash-merged approvals are off
  `main`. A sync is the PR form of that push and is skipped for the same
  reason.
- **program-north-star Decision 14 and a15 (b):** the sync is a merge
  commit of the program trunk and `main` with conflict-only fixes; a
  resolved conflict gets a tier-2 review of the resolution diff (sync gate
  point 5); a conflict the loop cannot resolve escalates. This design keeps
  all of it and leans on point 5 for the resolution.
- **`check.SyncHead` and `cleanMerge`** (`tools/workflow/check/merge.go`):
  the shape of a sync head (first parent the program trunk, second a commit
  of `main`, nothing else) and the clean-merge tree via `git merge-tree
  --write-tree`. CI's recognition of a sync reuses this code, so CI and the
  sync gate agree on what a sync is (FLOW-15).
- **FLOW-14's classifier** (`check/classify.go`, `workflowGuardChanged`,
  the profile's `rules.paths`, `rules.machinery`, `rule_jobs`): the
  definition of a guarded path, reused for what a sync's resolution may not
  edit.
- **FLOW-20's trunk split** (`process.md`, the program trunk bullet): a
  process item, and any change to the loop's rules or machinery, goes to
  `main`, never to a program trunk. So the program trunk side of a sync
  never edits a guarded path, and a sync conflict on one is not a routine
  resolution.
- **tier1-static-checks, LINT-04:** tier 1 runs on the PR's diff. Kept: the
  sync is still checked on its whole diff, by every other check.

## Decisions

1. **A sync PR is judged as the push its merge makes.** Tier 1 runs on the
   sync's full diff, as now, with `-no-design`. Every other check still
   runs over `main`'s changes as merged with the program trunk's, which
   catches what neither side had alone. The push to the program trunk
   after the merge already runs exactly this (`before..after`,
   `-no-design`); the PR now agrees with it before the merge instead of
   after.
   - *Rejected: judge only the sync's own resolution* (the head against the
     clean merge tree). Tier 1 diffs against a merge base with `HEAD`; the
     clean merge tree is no ancestor, and both parents are merge bases, so
     there is no base that yields the resolution alone. It would also stop
     the other checks from seeing `main`'s changes under the program
     trunk's code. The resolution's design question is already the tier-2
     review's (sync gate point 5), which judges FLOW-01's agent half: that
     the resolution is conflict-only and covered by the two sides' designs.
   - *Rejected: keep it, and label each failing sync `no-design`.* A
     confirmation with one possible answer, about 1 a week (**Program
     payoff**), and the loop cannot apply the label itself without widening
     its authority.
   - *Rejected: let the loop label the sync `no-design`.* It would let the
     loop clear a FLOW-01 finding on any PR it can label, which is wider
     than the sync.
2. **CI recognizes a sync by its shape, not its branch name.** A new
   stateless `wf sync-shape` decides it, from `check.SyncShape`, which
   `SyncHead` also uses. A PR is a sync when all hold:
   - the event is `pull_request`, the base ref starts with `program/`, the
     head is in this repository (not a fork), and the head ref matches
     `sync/program-<hex>`;
   - the PR head is a merge of exactly two parents; the first is an
     ancestor of the PR's base SHA; the second is an ancestor of
     `origin/<default branch>` and starts with the branch's `<hex>`;
   - the resolution, the paths where the head's tree differs from `git
     merge-tree --write-tree <first> <second>`'s tree (conflicted or not),
     has no guarded path (Decision 4).

   Anything else, including `wf sync-shape` exiting 2, is judged as any PR,
   as today: fail-closed, and the step log says which condition failed.
   - *Rejected: the head ref name alone.* Anyone who can push a branch can
     name it `sync/program-x` and carry new code past FLOW-01.
   - *Rejected: inline shell in `main.yml`.* It would duplicate
     `SyncHead`'s definition untested; `wf` is already built in this job by
     `enforcement-check.sh`.
3. **The enforcement comparison is skipped on a sync**, as on a push. Each
   of `main`'s enforcement changes was compared at its own PR against
   `main`'s tier 1, and Decision 4 keeps the resolution off every
   enforcement path, so the sync adds no enforcement change of its own.
   - *Rejected: run it with the sync's second parent as base.* It would
     compare `main`'s tier 1 against the program trunk's merged in, but the
     program trunk never edits enforcement (FLOW-20's trunk split), so
     there is nothing to compare; it costs a second tier 1 build per sync.
   - *Rejected: run it as now.* It fails on any approved `main` relaxation
     (**Problem**), and no label clears it.
4. **A sync resolution may not edit a guarded path.** A path is guarded
   when FLOW-14's classifier, built from the profile at either parent
   (`loop-profile.yaml`'s `rules.paths`, `rules.machinery`), calls it a
   rule or machinery path, or it is a workflow file whose change between
   the clean merge tree and the head touches a rule-running job
   (`workflowGuardChanged` with `rule_jobs`). `wf sync-program -head`
   refuses such a head with that reason, and `wf-publish` treats the
   refusal as a conflict it cannot resolve: it escalates under a15 (b)
   (`program-sync-<main sha>`, a `human` first step). Since the program
   trunk never edits these paths, the refusal should never fire on a
   correct sync; when it does, Jeff sees it.
   - *Rejected: let CI fall back alone.* The sync would fail CI on every
     tick without escalating.
   - *Rejected: allow it under the tier-2 review.* A loop-written edit to
     the checks that stop the loop, merged by the loop, is the authority
     FLOW-20 reserves to Jeff.
5. **FLOW-01 and FLOW-14 say so.** FLOW-01 names both exemptions (the push
   and the sync PR) and where the sync's resolution is judged; FLOW-14's
   CI sentence says the comparison does not run on a sync. Both are rule
   changes of a loop rule, made by this process item (LOOP-09, FLOW-14).
6. **The sync procedure needs one change only.** `wf sync-program` (the
   branch it names), `wf check -sync` (`SyncReady`: base, recorded head,
   shape, checks, containment, point 5) and the merge stay as they are:
   once CI judges the sync as a push, gate point 3 passes on its own.
   `wf sync-program -head` gains Decision 4's refusal, and `wf-publish`'s
   `sync` paragraph says what the refusal means. No state step: `wf
   sync-shape`, like `wf drops`, is a CI-only command the workflow README
   does not list.

## Questions

### Blocking

None.

### Assumed

- **Q1. How tier 1 judges a sync PR.** Options: (a) as its merge's push:
  full diff, every check, `-no-design` (Decision 1); (b) only the
  resolution against the clean merge tree; (c) unchanged, Jeff labels each
  failing sync. Proceeding with (a); confirm. Flagged: it departs from
  FLOW-01's label-only exemption, which Jeff applied by hand on #140, and
  it relaxes a check on a PR the loop merges (FLOW-20's reserved rules).
- **Q2. The enforcement comparison on a sync.** Options: (a) skipped, as on
  a push (Decision 3); (b) run with the second parent as base; (c) run as
  now. Proceeding with (a); confirm. Flagged: it narrows where FLOW-14's
  automated check runs.
- **Q3. How CI recognizes a sync.** Options: (a) by its shape through `wf
  sync-shape`, sharing `SyncHead`'s code (Decision 2); (b) by the head ref
  name; (c) inline shell in `main.yml`. Proceeding with (a); confirm.
- **Q4. A resolution on a guarded path.** Options: (a) `wf sync-program
  -head` refuses it and the sync escalates (Decision 4); (b) CI falls back
  alone; (c) allowed under the tier-2 review. Proceeding with (a); confirm.
- **Q5. The rule text.** Options: (a) amend FLOW-01 and FLOW-14 to name
  the push and sync exemptions (Decision 5); (b) leave the wording, as the
  push exemption has been. Proceeding with (a); confirm.

## Design

### The tier1 job

The `tier1` job gains, before the `tier1` step:

```yaml
      - name: program sync
        id: sync
        if: >-
          github.event_name == 'pull_request' &&
          startsWith(github.event.pull_request.base.ref, 'program/') &&
          startsWith(github.event.pull_request.head.ref, 'sync/program-') &&
          github.event.pull_request.head.repo.full_name == github.repository
        env:
          BASE: ${{ github.event.pull_request.base.sha }}
          HEAD_SHA: ${{ github.event.pull_request.head.sha }}
          BRANCH: ${{ github.event.pull_request.head.ref }}
          DEFAULT: ${{ github.event.repository.default_branch }}
        # A sync carries main's merged PRs, each judged on its own PR, and a
        # conflict-only resolution its tier-2 review judges (FLOW-01, FLOW-20).
        # Not a sync, or wf cannot say: judged as any PR.
        run: |
          (cd tools/workflow && go build -o ../../dist/wf ./cmd/wf)
          if ./dist/wf sync-shape -repo . -base "$BASE" -branch "$BRANCH" \
              -head "$HEAD_SHA" -trunk "origin/$DEFAULT"; then
            echo sync=true >> "$GITHUB_OUTPUT"
          fi
```

The `tier1` step's `FLAGS` adds `steps.sync.outputs.sync == 'true'` to its
`-no-design` condition, and its comment names the sync. The **main's
enforcement** step's `if` adds `&& steps.sync.outputs.sync != 'true'`, and
its comment says a sync is skipped as a push is. `actions/checkout` with
`fetch-depth: 0` already has the PR head (the merge ref's second parent)
and `origin/<default branch>`.

### `check.SyncShape` and `wf sync-shape`

`check.SyncShape(repo, base, branch, head, trunk string) (resolution
[]string, err error)` checks Decision 2's git conditions and returns the
resolution's paths, or why the head is not a sync. It uses the
`syncBranchRe`, parent and `merge-tree` code `syncHead` and `cleanMerge`
use now, moved into shared helpers; `syncHead` calls it with `base` =
`origin/<program trunk>` (where it keeps its stricter equal-first-parent
check) and `trunk` = `origin/<trunk>`. A second function,
`GuardedResolution(repo, head, paths)`, returns the paths Decision 4
guards, from the classifier at each parent (`classifierAt`'s profile
reading at a given revision) and `workflowGuardChanged` between the clean
tree and the head.

`wf sync-shape -repo DIR -base SHA -branch REF -head SHA -trunk REF` takes
no `-state`. Exit 0 prints `sync <head12> merges <second12> into <first12>`;
exit 1 prints each failed condition (UX-04: what failed and that the PR is
judged as any PR); exit 2 is a git or usage error.

### `wf sync-program -head`

After `SyncHead` passes, `recordSync` calls `GuardedResolution` and, when
it returns paths, refuses: `<head> resolves <paths>, which FLOW-14 guards:
a sync resolution may not edit them; escalate (FLOW-20)`, recording
nothing. The usage line and the doc comment say so.

### `wf-publish`'s sync paragraph

One sentence after "it refuses a head that is not that merge and nothing
more": "and one whose resolution edits a rule, machinery or rule-running
workflow path (FLOW-01); that refusal is a conflict you cannot resolve, and
escalates as below."

### The rule and contributor text

FLOW-01 and FLOW-14 as **Rule changes** gives. `CONTRIBUTING.md`'s **No
design** bullet adds: "A push, and a program sync's PR, are not judged on
it (FLOW-01)."

### Every rule, machinery path and configuration key the change reaches

- **Rules changed:** FLOW-01 (its automated half's scope), FLOW-14 (where
  the enforcement comparison runs). See **Rule changes**.
- **Rules relied on, unchanged:** FLOW-08 and the sync gate
  (`SyncReady`'s points 1 to 5: unchanged code; point 3 now passes on a
  sync without a label), FLOW-20 (Decision 14, a15 (b), the trunk split),
  LINT-02 and LINT-04 (tier 1 still runs on the PR's diff, every other
  check included), PROFILE-01 (`rules.paths`, `rules.machinery`,
  `rule_jobs` read, not changed), FLOW-17 (no push or merge path changes),
  LOOP-09.
- **CI:** `.github/workflows/main.yml`, the `tier1` job: a new `program
  sync` step, the `tier1` step's `FLAGS`, the **main's enforcement**
  step's `if`. A rule-running job (FLOW-14's CI paths).
- **Machinery paths:** `tools/workflow/check/merge.go` and
  `merge_test.go` (`SyncShape`, `GuardedResolution`, `syncHead` reusing
  them); `tools/workflow/cmd/wf/main.go` (`wf sync-shape`, `recordSync`'s
  refusal, usage) and `trunk_test.go`; `.claude/agents/wf-publish.md` (one
  sentence).
- **Rule paths:** `docs/development/process.md` (FLOW-01, FLOW-14).
- **Book:** `CONTRIBUTING.md` (one sentence).
- **Configuration keys:** none added or changed. Read: `loop-profile.yaml`
  `rules.paths`, `rules.machinery`, `rule_jobs`; the event fields
  `pull_request.{base.ref, base.sha, head.ref, head.sha, head.repo}` and
  `repository.default_branch`.
- **Not reached:** `config.yaml` (no limit, budget, lease or
  `runaway_factor`), every `.claude/settings*.json` allow or deny list,
  `enforcement-check.sh`, `tools/tier1/`, `wf check -sync` and the merge
  command, every push and tag path, the release workflows, the workflow
  branch.

## Program payoff

Figures from `origin/main`'s first-parent history 2026-10-26..11-23 (80
commits, 51 PRs) and the two syncs so far:

- **A label that decides nothing.** A sync fails FLOW-01 when its range
  has code and no design. `main` landed 6 code PRs without a design in
  those 4 weeks (#94, #97, #111, #137, #138, #139), 4 of them in the last
  2 weeks: about **2 a week** while the program runs, since FLOW-20 sends
  every process item and fix to `main`. A range is design-free when such a
  PR lands alone or beside other `no-design` PRs between two syncs; of the
  two syncs so far, one (#140, range #138 and #139) was. Counting half the
  `no-design` PRs: about **1 interaction a week** removed, each a
  confirmation with one possible answer (Jeff labels the sync), and each
  also a stall: the program trunk does not get `main` until he does, and
  every program item's refresh waits.
- **A failure no label clears.** 27 of the 80 commits touch
  `tools/tier1/` or `tools/workflow/check/`. Each makes the next sync run
  the enforcement comparison; one whose approved rule change drops a
  finding fails it with no remedy but Jeff's (an escalation, a design for
  the sync, or a merge outside the loop). Not yet observed, so not
  counted.
- **Cost.** One process item: about 3 steps to implement (`SyncShape` and
  `wf sync-shape` with tests; `main.yml`; the rule, `CONTRIBUTING.md` and
  `wf-publish` text), Review, and one design review by Jeff, which this
  design needs anyway (P2). Against 1 interaction a week, it pays back
  within the first weeks of the program, which runs for months
  (componentization.md's phases).

## Defaults and overrides

- **A program sync's PR is judged without FLOW-01's automated half and
  without the enforcement comparison** (new default). It is the obvious
  choice (GLOBAL-09): its merge's push is already judged so, and every
  change it carries was judged on its own PR. Override: none; the
  `no-design` label still works on any PR, and nothing can make a sync
  judged as new work except failing its shape.
- **Fallback: judged as any PR** (GLOBAL-10, GLOBAL-11). Triggered when
  the PR is not a sync by its shape, its resolution edits a guarded path,
  or `wf sync-shape` exits 2. The `program sync` step's log names the
  failed condition and says the PR is judged as any PR; the `tier1` step
  then reports FLOW-01 as today. This is today's behavior, so the fallback
  is fail-closed.
- **Refusal: a guarded resolution is not recorded** (`wf sync-program
  -head`), and the sync escalates. Seen as the command's error and the
  escalation item.

## Cross-driver impact

None: no driver, executor or relay code is reached; CI's other jobs and
the `changes` classifier are unchanged.

## Rule changes

### FLOW-01 (amend), `docs/development/process.md`

Current, the last paragraph:

> The automated part checks that a non-exempt PR links a design document
> with valid front-matter; the agent part judges whether the design
> actually covers the change.

Proposed:

> The automated part checks that a non-exempt PR links a design document
> with valid front-matter; the agent part judges whether the design
> actually covers the change. Two kinds of change carry only changes
> already judged on their own PRs, and are not judged again: a push to a
> trunk or a program trunk, and a program sync's PR (FLOW-20), which CI
> recognizes by its shape: a merge of the program trunk and a commit of
> the trunk on its `sync/program-<sha>` branch, whose conflict resolution
> edits no path FLOW-14 guards. The sync's resolution is judged by its
> tier-2 review (FLOW-08, FLOW-20).

Severity MUST and check automated + agent-judgment, before and after.
Enforcement: `main.yml`'s `program sync` step and the `tier1` step's
`FLAGS` pass `-no-design` on a sync as on a push; `wf sync-shape` and
`check.SyncShape` decide the shape; `wf sync-program -head` refuses a
guarded resolution, so no sync the loop records has one. Why: #140.

### FLOW-14 (amend), `docs/development/process.md`

Current, in the automated part's paragraph:

> When a branch changes `tools/tier1/` or `tools/workflow/check/`, Review
> and CI also run tier 1 built from the merge base's code with the
> branch's configuration (`tools/tier1/rules.yaml` and `golangci.yml`)
> against the branch, and every finding it reports that the branch's tier
> 1 no longer reports must be on a rule an approved `rule_changes` entry
> names; Review also runs the merge base's `wf check`.

Proposed, one sentence added after it:

> A program sync's PR (FLOW-01) is not compared so: each enforcement
> change it carries was compared on its own PR, and its resolution edits
> none of these paths.

Severity MUST and check automated + agent-judgment, before and after.
Enforcement: the **main's enforcement** step's `if` skips a sync;
`enforcement-check.sh` is unchanged. Why: a sync would fail on any
approved `main` relaxation (**Problem**).

## Validation plan

- **Tier 0:** `merge_test.go` builds temporary repositories for
  `SyncShape` and `GuardedResolution`: a clean sync; a resolved conflict
  on an ordinary path (a sync, with that path as resolution); a resolved
  conflict under `tools/tier1/`, `docs/development/` and `.claude/`, and
  one in a workflow's tier1 job (guarded); a workflow edit outside a
  rule-running job (not guarded); a single-parent head; a first parent not
  in the base; a second parent not in the trunk; a branch whose hex does
  not prefix the second parent; an extra commit on top of the merge.
  `trunk_test.go`: `wf sync-shape`'s exit codes and messages, and `wf
  sync-program -head` refusing a guarded resolution and recording nothing.
  The existing `SyncHead` and `SyncReady` tests pass unchanged.
- **Tier 1:** this item's PR runs the enforcement comparison (it changes
  `tools/workflow/check/`) and must drop nothing.
- **Tier 2:** none. CI on the first sync after the merge: its `program
  sync` step reports `sync`, `tier1` runs with `-no-design`, and **main's
  enforcement** is skipped. A design-free range is not needed to see it;
  the step's output shows the decision on any sync.
- **Native:** none (`native_validation: []`).

## Implementation plan

1. `check.SyncShape` and `GuardedResolution` in
   `tools/workflow/check/merge.go`, `syncHead` and `cleanMerge` reusing
   them, with `merge_test.go` cases (FLOW-01, FLOW-14, FLOW-15).
2. `wf sync-shape` and `recordSync`'s refusal in
   `tools/workflow/cmd/wf/main.go`, usage and doc comments, with
   `trunk_test.go` cases (UX-04).
3. `main.yml`'s `program sync` step, `FLAGS` and the enforcement step's
   `if`, with their comments (FLOW-01, FLOW-14).
4. FLOW-01's and FLOW-14's wording as approved; `CONTRIBUTING.md`'s
   bullet; `wf-publish`'s sentence (FLOW-14, UX-01).
5. `make test`, `make lint`, `make enforcement` (FLOW-17).

## Design review

## Consolidation

## Rollback notes

## Archive summary
