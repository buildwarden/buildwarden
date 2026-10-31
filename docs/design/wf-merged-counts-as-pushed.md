---
feature: wf-merged-counts-as-pushed
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-08, FLOW-12, FLOW-16, FLOW-17, TEST-01]
concepts: [workflow, done-when, merge-detection]
related: [workflow-streamline.md, queue-summary.md]
---

# A merged branch satisfies Publish's checks

## Problem

Twice on 2026-10-31 Jeff merged a PR as soon as its CI went green, before the
loop recorded the step that opened it (`queue-summary` #78 at `x2-implement`,
`tier1-static-checks` #79 at `f6-open-pr`). GitHub then deleted the branch and
`main` moved past it, and the done-when checks of the step that was still
open could no longer pass:

- `branch pushed` compares the local branch with a remote branch that no
  longer exists;
- `current main merged` asks whether the branch contains `origin/main`, which
  is now false, since `origin/main` contains the branch;
- `design archived` reads the design on the branch, which only exists locally.

Both items were advanced by hand, with a history note. Merging early is the
human's call and should be the fast path, not a manual repair.

## Prior art

- `workflow-streamline.md` made the merge the sign-off (FLOW-08) and taught
  `merged()` to find a merge after the branch is deleted, by the PR's merge
  commit. This applies the same idea one phase earlier.

## Decisions

1. **Once merged, the merge supersedes Publish's checks.** In feature phase 6
   and process phase 2, when `merged()` passes, the checks become: merged by a
   human (FLOW-08), and the design archived with its summary on `origin/main`
   (FLOW-12). Pushed, main-merged and PR-green no longer apply: the human merged
   that PR, and CI on it is theirs to have judged. Rejected: requiring the
   recorded `ci: green` as well (a merge without it is still the human's
   decision, and the queue may not have caught up).
2. **Nothing else changes.** Before the merge, the checks are exactly today's.
   Merge (phase 7 / x3) and Close then pass as they do now.

## Questions

### Blocking

None.

### Assumed

None.

## Design

`check.Phase`: in feature phase 6 and process phase 2, run `merged()` first,
quietly; if it passes, report it plus `archivedOn("origin/main")` and stop.
Otherwise run today's checks.

## Cross-driver impact

None (`drivers: []`).

## Validation plan

Tier 0 (TEST-01): in `check_test.go`, a feature in phase 6 whose branch was
merged into `main` and then deleted, with the PR's merge commit on `main`,
passes; the same before the merge still requires push, main-merged and a green
PR; the process item's phase 2 likewise.

## Implementation plan

1. `check/check.go`: the merged short-circuit for feature phase 6 and process
   phase 2 (FLOW-03, FLOW-08, FLOW-12).
2. Tests as in the Validation plan (TEST-01).

## Design review

Pending.

## Consolidation

Filled in at Publish.

## Rollback notes

## Archive summary

Filled in at Publish.
