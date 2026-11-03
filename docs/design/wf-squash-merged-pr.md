---
feature: wf-squash-merged-pr
status: archived
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-08, FLOW-09, FLOW-13, FLOW-16, TEST-03]
concepts: [workflow, merge-detection, squash-merge, branch-cleanup, native-gaps, backlog]
related: [wf-merged-counts-as-pushed.md]
---

# wf recognises squash merges; backlog gaps can be deferred

## Problem

**Squash merges.** firecracker-driver's PR #90 was squash-merged to `main` as
`1ef5547 Firecracker driver (#90)`, and GitHub deleted its branch. The queue
could not move past Publish. `f6-open-pr-r2` was released, and on 2026-11-03
Jeff marked f6, f7 and f8 done by hand. While it was stuck, it held the single
PR slot, so http-body-path-drift could not publish.

The cause is `check.merged()` in `tools/workflow/check/check.go`. It walks
`refs/heads/<branch>` and then `refs/remotes/origin/<branch>`. At the first
ref that exists, it asks whether that ref is an ancestor of `origin/main` and
returns the answer. It falls back to searching `main`'s log for the PR number
only when neither ref exists. A squash merge never makes the branch head an
ancestor of `main`, so the check fails whenever a ref is still around. That
covers a cloud checkout that still has the local branch, and a remote-tracking
ref that has not been pruned. `mergedEarly()` (phase 6, process phase 2),
`f7-merge`/`x3-merge` and `closed()` (phase 8, process phase 4) all go through
`merged()`, so all of them fail the same way. PR #91 was squash-merged too
(`0728b26 ... (#91)`), so the next tick's `x3-merge` will hit this as soon as
the cloud checkout holds `feat/bootstrap-firecracker-row`.

Close has a second problem: FLOW-16 deletes a merged branch only with
`git branch -d`, never `-D`, and `-d` refuses a squash-merged branch. Even
with merge detection fixed, Close cannot delete the local branch, and per
FLOW-16 it would escalate.

**Deferred gaps.** `backlog/native-gaps.yaml` entries are `open | done`.
`firecracker-distro-firewalld` needs a host that does not exist yet. Each push
to a waiting branch woke windows-desktop for it, three runs in a row, and
each run skipped it. On 2026-11-03 Jeff set it to `status: deferred`. The gate
(`hosts.go`) wakes only for `open`, so the wake-ups stop. But `deferred` is
undocumented, `wf validate` does not check gap statuses, and a deferred gap
appears nowhere in `QUEUE.md`, so it could be forgotten.

## Prior art

- `wf-merged-counts-as-pushed` (eca1a46) added `mergedEarly()` and the
  PR-number search for a branch GitHub has deleted. This design extends the
  same search to a branch that still exists but was squashed.
- FLOW-16 already allows proof by "the PR's merge commit". A squash commit is
  the PR's merge commit for a squash merge. What a squash cannot show is that
  the branch's content is on `main`, which is what FLOW-16's safety rule is
  for.

## Decisions

1. **`merged()` accepts a squash merge** (question a1). For each existing ref,
   the check passes when the ref is an ancestor of `origin/main` (as today),
   or when `origin/main` has a commit naming the PR (the existing
   `(Merge pull request |\()#N` search) whose tree equals the ref's tree. The
   tree comparison is the proof: the squash produced exactly the branch's
   content, so deleting the branch loses nothing. With no ref, the PR search
   alone passes, as today, because GitHub deletes the branch only on merge.

   Checked against the real squash merges: the trees of PR #90's head and
   `1ef5547` are identical (`b855cee`), and so are PR #91's head and `0728b26`
   (`bfdb82e`). Publish merges current `main` into the branch just before the
   PR (f6's "current main merged"), which makes equal trees the normal case.

   If the PR commit exists but the trees differ (`main` moved under the
   branch between its last main-merge and the squash), the check fails and
   names both commits, and the step escalates as any failed merge check does.

   *Rejected:* trusting the PR-number commit alone when a ref exists. That
   could delete commits pushed to the branch after the squash. Also rejected:
   requiring merge commits through the repository setting alone, because a
   human can still pick squash in the GitHub UI.

2. **FLOW-16 names the squash case** (question a2). The rule text becomes:
   contained in trunk means `git branch --merged`, the PR's merge commit, or,
   for a squash merge, a commit on trunk naming the PR whose tree equals the
   branch head's tree. In that last case Close deletes the local branch with
   `git branch -D`, the one use of `-D` the rule allows, and only after
   `wf check` passed. The `workflow-tick` Close step says the same.

3. **`deferred` is a gap status** (question a3). Backlog gaps are
   `open | deferred | done`.
   - `deferred` needs a dated `note` saying what it waits on and who
     provides it.
   - `wf validate` rejects any other status, and a deferred gap without a
     note.
   - The gate and `workflow-native` skip deferred gaps.
   - `QUEUE.md` does not list deferred gaps. They stay in
     `backlog/native-gaps.yaml`, where their notes say what they wait on.
   - The status goes back to `open` once the wait is over, by hand or
     through `workflow-human`.

## Questions

### Blocking

None.

### Assumed

- **a1. How a squash merge counts as merged.** Proceeding with: a commit on
  `origin/main` naming the PR whose tree equals the branch ref's tree; confirm.
  Options:
  - (a) the PR commit plus tree equality;
  - (b) the PR commit alone, even when a ref exists;
  - (c) no wf change: set the repository to allow merge commits only, and
    record a squash by hand.

  Recommended: (a).
- **a2. FLOW-16 and `-D`.** Proceeding with allowing `git branch -D` only for a
  squash-merged branch proved by a1(a); confirm. Options:
  - (a) allow it in that case;
  - (b) keep "never `-D`" and always escalate a squash-merged local branch to
    Jeff.

  Recommended: (a).
- **a3. Deferred gaps.** Proceeding with `deferred` as a third status that
  needs a note and is skipped by the gate, without a QUEUE.md listing (Jeff,
  r1; this was option (b)); confirm. Options:
  - (a) as stated;
  - (b) `deferred` without the Needs Jeff listing;
  - (c) no new status: re-home the entry to a `host:` nobody serves.

  Recommended: (b), Jeff's choice at r1.
- **a4. Holds nothing.** The change reaches the running loop only when merged,
  and no step in flight depends on the old behaviour; confirm.

## Design

| Site | Change |
|---|---|
| `tools/workflow/check/check.go` `merged()` | per ref: ancestor, **or** PR commit on `origin/main` with an equal tree; failure detail names the PR commit and both trees |
| `tools/workflow/check/check_test.go` | squash merge with the local ref present passes; squash with differing trees fails; a ref pushed past the squash fails; the existing deleted-branch and `#50`/`#5` cases stay |
| `tools/workflow/queue/hosts.go` | validate gap status ∈ {open, deferred, done}; deferred needs a note (the gate already wakes only for `open`) |
| `docs/development/process.md` FLOW-16 | the squash case and its single `-D` |
| `.claude/skills/workflow-tick/SKILL.md` Close | squash-merged branch: `-D` after `wf check` passes |
| `.claude/skills/workflow-native/SKILL.md` | backlog: `open` entries only; deferred ones are not retried |
| workflow branch `README.md` | gap statuses (follow-up commit on `workflow` after merge) |

## Validation plan

Tier 0: the `check` and `queue` tests above, `make test` and `make lint` (the
`tools/workflow` module). No native validation: nothing here runs on a host.

## Implementation plan

1. `merged()` with tree equality, plus tests (FLOW-08, FLOW-16).
2. Gap status validation (status set, note on deferred), plus tests
   (FLOW-09, TEST-03).
3. FLOW-16 text, then the `workflow-tick` Close and `workflow-native` backlog
   wording (FLOW-14).
4. `make test`, `make lint`. Archive the design, then open the PR (x2).
5. After merge: the workflow branch `README.md` gap schema line.

## Design review

- **r1, 2026-11-03: approved** (Jeff, attended session, FLOW-19). Approved at
  `68484a6`, with one change folded in and no second review.
  - a1 (the PR commit plus equal trees), a2 (`-D` only for a proven squash)
    and a4 (holds nothing) are confirmed as recommended.
  - a3 is changed: `deferred` is a gap status with a required note, skipped
    by the gate and by native sessions, but QUEUE.md does not list deferred
    gaps (option (b)). Decision 3, the Design table (no `summary.go` change)
    and task 2 are updated to match.

## Consolidation

Branched from `origin/main` at `0728b26` (#91), still `main`: nothing landed
to reconcile. #90 and #91, the two squash merges this design was checked
against, are both on `main` at the fork.

- **FLOW-15 sweep:** one helper added (`refMerged`), reached from `merged()`,
  which `mergedEarly()`, the merge steps and `closed()` already share, so the
  squash proof is in one place. `validateGaps()` reuses `oneOf()`. `overview.md`
  describes the product components, not `tools/workflow`, and is unchanged.
- **Found outside the design:** a stale `refs/remotes/origin/<branch>`, left
  unpruned at a head older than the PR's (the tick fetches without
  `--prune`), has a tree no PR commit matches, so `merged()` fails safe and
  the step escalates. Proposed as a follow-up in the PR: `git fetch --prune
  origin` before the Merge and Close checks, or ignore the remote ref once
  `ls-remote` shows the branch is gone.
- **Task 5** (the workflow branch `README.md` gap statuses) is made on
  `workflow` after merge; the PR gives the text.

## Rollback notes

Revert the PR. `merged()` then fails on a squash-merged branch whose ref is
still around, as before, and a `deferred` gap passes `wf validate` unchecked.

## Archive summary

Approved at design review r1 (2026-11-03, Jeff); implemented and published
the same day as a process item. The PR to `main` carries this design.

- **Built.** `check.merged()` checks every branch ref still around, local and
  remote. Each must be an ancestor of `origin/main` or, for a squash merge,
  match the tree of a commit on `origin/main` naming the PR (`refMerged`).
  With no ref left, the PR commit alone passes, as before. A failure names
  the ref's tree and each PR commit's tree. Tests: `TestMergedBySquash`,
  `TestSquashOfAnOlderHeadFails`, `TestCloseAfterASquash`.
  `queue.validateGaps()`: gap status is `open | deferred | done`, and
  `deferred` needs a note (`TestValidateGapStatus`,
  `TestGateSkipsDeferredGaps`). FLOW-16 names the squash proof and allows
  `git branch -D` for that case alone, after `wf check` passes; the
  `workflow-tick` Close step and the `workflow-native` backlog step say the
  same.
- **Decisions taken:** a squash counts as merged only with an equal tree (a1);
  `-D` only for a branch proved that way (a2); `deferred` is a third gap
  status with a required note, skipped by the gate and native sessions, not
  listed in `QUEUE.md` (a3, Jeff's option (b) at r1).
- **Rejected:** trusting the PR-number commit alone while a ref exists (it
  could delete commits pushed after the squash); allowing merge commits only
  through the repository setting (a human can still squash in the UI);
  keeping "never `-D`" and escalating every squash; re-homing a waiting gap to
  a host nobody serves; listing deferred gaps under Needs Jeff.
- **Verified live:** with the branch build, bootstrap-firecracker-row's
  `x3-merge` (PR #91, squashed as `0728b26`) passes, read-only.
- **For a future design touching `merge-detection`, `squash-merge`,
  `branch-cleanup`, `native-gaps` or `backlog`:** equal trees are the normal
  case only because Publish merges current `main` into the branch just before
  the PR; a squash after `main` moved under the branch fails and escalates.
  The check reads whatever refs are present, so a stale unpruned remote ref
  also fails it (the open follow-up). A new gap status must be added to
  `validateGaps()`, the gate in `hosts.go`, and the workflow branch
  `README.md`.
