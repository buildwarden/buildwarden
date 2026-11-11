---
feature: next-skips-merge-wait
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-04, FLOW-05, FLOW-08, FLOW-09, FLOW-13, FLOW-14, FLOW-17,
  FLOW-19, GLOBAL-01, GLOBAL-09, GLOBAL-10, GLOBAL-11, TEST-01, TEST-02]
rule_changes: []
concepts: [workflow, queue, queue-order, wf-next, merge-wait, process-item,
  pr-slot, follow-up]
related: [regression-fixes-follow-up.md, design-principles.md,
  rule-running-line-match.md, workflow-streamline.md, wf-squash-merged-pr.md]
---

# wf next skips a merge step that waits on the human

## Problem

On 2026-11-10 the cloud tick `cloud/20261110T105938Z` finished
design-principles' and rule-running-line-match's `x2-implement` and opened
PRs #104 and #105. From then on, both items sat at `x3-merge`. `wf next` kept
returning `design-principles x3-merge` to the cloud. That step's done-when
fails until Jeff merges. So hyperv-relay-per-build-channel's ready
`f1-design-r3`, a class-3 fix, was never offered, and the tick ended early
(digest 2026-11-10, lines 50-51). The tick filed this item under FLOW-09 as a
follow-up of regression-fixes-follow-up (#103), which introduced the class
order.

The rule already says this should not happen. FLOW-05 (`process.md:180-182`)
puts an in-progress process item in class 1 "except while it waits for the
human's merge". FLOW-03's phase-7 row (`process.md:116`) says "The loop
confirms it; there is no step for the human to claim". The workflow branch's
`README.md:100` says `f7-merge` "takes no claim or budget". The code gets
part of this right. `Class()` drops a merge-waiting item out of class 1
(`queue.go:254-266`). But it lands in its own class, where `Next` still
offers its merge step like any other cloud step:

- `Next` walks `Ordered()` and returns the first in-progress item with a ready
  step (`next.go:93-110`).
- `ready()` offers every pending, phase-reached step whose needs are done and
  whose affinity matches. It skips a step only on affinity, a lease held by
  another worker, or `cloudHold` (`next.go:164-184`). Neither `cloudHold`
  (`next.go:193-214`) nor anything else in that path knows that a merge step
  can only pass after the human acts.
- Both merge steps are cloud template steps: `{"merge", "cloud", ...}` for the
  feature round's phase 7 (`phases.go:61-62`) and the process round's phase 3
  (`phases.go:72`).

So an unmerged `x3-merge` heads class 2 and starves classes 3 and 4. An
unmerged `f7-merge` heads its class (3 or 4, by `follows`). Within that class,
in-progress items come before queued ones (`queue.go:205-207`). That starves
every lower-priority in-progress item of its class, every queued item of its
class, and every item of a lower class.

The tick confirms merges itself before its loop
(`.claude/skills/workflow-tick/SKILL.md:61-65`): `wf done` on each `f7-merge`
and `x3-merge`, with no claim. That works because `wf done` checks no lease
(`cmd/wf/main.go:252-266`). Its dispatch table has no row for a merge step
(`SKILL.md:80-91`). So when `wf next` returns one inside the loop, the
session has no defined move. It either judges its way past the step, or it
ends.

**Features had the same problem; sessions stepped past it by judgment.**
Replaying the workflow branch with a `wf` built from `beb51ba` shows this
(`wf next -worker cloud/x` on a `git archive` of each commit):

- `f177cdd` (2026-11-04 11:18, after http-body-path-drift's `f6-open-pr`)
  returns `http-body-path-drift f7-merge`. That tick claimed
  build-link-subnet's `f1-design-r2` two minutes later and went on to
  `f2-plan-r2`. The session stepped over the merge step because the tick's
  step 2 says to "leave it pending". `wf` did not tell it to.
- `cb67d90` (2026-11-06 11:13, after build-link-subnet's `f6-open-pr`)
  returns `build-link-subnet f7-merge`. That run wrote its digest 20 seconds
  later. The only other cloud step was security-suite's `f1-design`, which
  every run skipped by judgment until Jeff scoped it. So nothing was visibly
  starved.
- `9649de9` (2026-11-10, the incident) returns `design-principles x3-merge`.

Before #103, `Next` did the same: it returned the first in-progress item with
a ready step, in priority order (`49e5354^:tools/workflow/queue/next.go:85-106`).
The tests lock the behavior in. `TestNextTakesImplementationFirst` expects
`proc x3-merge` once the class-1 feature blocks (`queue_test.go:217-219`).
`TestPRSlotIsSerial` expects `a f7-merge` (`queue_test.go:570-572`). Until
11-10, unmerged merge steps were rare, short-lived, and usually alone, so
judgment covered them. With two process PRs waiting and a ready fix behind
them, judgment did not cover them.

## Prior art

- **regression-fixes-follow-up** (#103), Decision 2
  (`docs/design/regression-fixes-follow-up.md:166-187`), defined "in
  implementation" to exclude the merge phases. It explicitly rejected "make
  `ready` skip an unmerged `*-merge` step" because the class "already handles"
  it (`:184-187`). That holds only between classes. Within the item's own
  class, and for every class below it, the merge step still comes first. This
  design takes up the rejected alternative, in the form a1 recommends: skip
  every merge step, not only an "unmerged" one, so that `wf next` needs no
  git.
- **workflow-streamline** and the workflow branch's README made the merge a
  cloud step that "passes once the human has merged, takes no claim or
  budget". This design makes `wf next` agree with that line.
- **wf-squash-merged-pr** (#92) owns the merged check itself
  (`check/check.go:251-317`). It is unchanged.
- **design-principles** (#104): GLOBAL-11 prefers predictable behavior over
  judgment-based workarounds. Today's "the session decides to step past it"
  is the kind of unwritten path that rule rejects.
- **FLOW-05's blocking list** (`process.md:193-197`) names where a worker
  moves to the next item: a `human` or `host` step, an exhausted budget, or
  the PR slot. The human's merge is another such point. The rule just never
  said so.

## Decisions

1. **`wf next` never offers a merge step. It reports the item as waiting on
   the human's merge.** A merge step is a template step named `merge` in its
   round's merge phase, in any `-r<n>` form: `f7-merge` (feature phase 7)
   and `x3-merge` (process phase 3), parsed by `stepTemplates`
   (`phases.go:120-134`). `ready()` (`next.go:156-190`) skips such a step for
   every worker, before the affinity test. It adds
   `the human's merge of PR #<n> (FLOW-08)` to the item's wait reasons, using
   `prRef` (`summary.go:94-99`), or `no PR recorded` when there is no PR.
   `Next` then records the item as `blocked` and walks on (`next.go:111-112`),
   exactly as for a human step. `NextFor` goes through `ready()` too
   (`next.go:270-273`), so an attended `-feature` run on a merge-waiting item
   gets `none` and the reason (a4). Nothing about "merged or not" is decided
   in `wf next`. The merge step is the only step left in its phase, so while
   it is pending the item has nothing else to offer.
   - **Rejected: `wf next` runs the merged check.** `Next` is a pure
     function of the queue state. It takes no `-repo` and runs no git
     (`cmd/wf/main.go:198-215`). The check needs a fetched `origin/main` and
     the PR's commits (`check.go:251-317`). The native gate runs `Next` for
     every host on every wake (`hosts.go:200`). Running git there adds a
     fetch dependency and a slow path to every call, for no gain: the tick
     already confirms merges once per run.
   - **Rejected: offer merge steps only to a confirming call** (a `-merges`
     flag, or a separate `wf merges` listing). The tick's step 2 already
     lists the items by phase and runs `wf done` on each. A second mode of
     `wf next` would be a new surface that duplicates that loop. Recorded as
     a1 (c).
   - **Rejected: make the merge step `human`.** FLOW-03 says there is no
     merge step for the human to claim (`process.md:116`). A human step is
     also offered early (`next.go:52-54`), and workflow-human would list it
     beside the design reviews.
   - **Rejected: a new class (0 or 5) for merge-waiting items.** Order only
     decides which ready step comes first. A blocked item's place in the
     order does not matter to `Next`. `Ordered()` also orders QUEUE.md's
     rows (`summary.go:141-171`), which would move for no reason.
   - **Rejected: record "merge checked, not merged" in the queue file at
     step 2.** That is state that goes stale the moment Jeff merges. It
     would also need a reset rule, plus a commit on every tick that finds
     nothing.

2. **Merges are confirmed only by the tick's step 2, unchanged.** `wf done`
   on the merge step, with no claim, is the one way it completes. It runs
   the phase's done-when (`check.go:77-78`, `:111-112`) and moves the item
   to Close. Step 2 gains one clause saying `wf next` never offers these
   steps, so a session reading the loop knows there is no other path. The
   workflow branch's `README.md:100` line and its `wf next` paragraph
   (`README.md:148-152`) say the same, at x4-close.
   - **Rejected: have `wf claim` refuse a merge step.** Nothing claims one
     today. The incident had no claim commit, and step 2 says no claim. That
     hardening is not needed for this fix. Recorded as a3.

3. **FLOW-05 names the human's merge as a blocking point.** In the
   sentence at `process.md:193-197`, "until it blocks: on a `human` or
   `host:<name>` step, an exhausted budget, or the PR slot" becomes "until
   it blocks: on a `human` or `host:<name>` step, an exhausted budget, the
   PR slot, or the human's merge (FLOW-08)". Class 1's "except while it waits
   for the human's merge" (`:180-182`) stays. That clause says where the
   item sorts, and the new words say that it yields. This change goes
   through the process route (FLOW-03, FLOW-14): this item's design review
   and Jeff's merge. `rule_changes` stays empty. Recorded as a5.

## Questions

### Blocking

None.

### Assumed

- **a1. How `wf next` knows "not merged"** (Decision 1). Proceeding with
  (a): it never offers a merge step at all. Options:
  - (a) Never offer `f7-merge` or `x3-merge` from `wf next`. The tick's step
    2 confirms merges. `wf next` stays git-free. A merged PR is seen at the
    next tick's step 2, which is no later than today, since the loop could
    never pass a merge step mid-run anyway.
  - (b) `wf next` runs the merged check. This needs `-repo` and a fetched
    `origin/main` on every call, including each native gate wake. It mixes
    checks into ordering, and the gate gets slower.
  - (c) Offer merge steps only to a confirming call (`wf next -merges`).
    This is a second mode that duplicates step 2's loop, and one more
    surface to document (UX-01).

  Recommended: (a).
- **a2. Which workers get the merge-wait reason** (Decision 1). Proceeding
  with (a): the skip comes before the affinity test, so every worker sees the
  same reason. Options:
  - (a) Every worker, cloud, human and host. Native digests then say
    "waiting on the human's merge of PR #104 (FLOW-08)" instead of today's
    "cloud step x3-merge" (digest 2026-11-10, lines 96 and 130).
  - (b) Cloud only (inside `Kind(worker) == "cloud"`, beside `cloudHold`).
    Hosts and humans keep "waiting on cloud step x3-merge", which misleads
    them about who the item waits on.

  Recommended: (a).
- **a3. Whether `wf claim` refuses a merge step** (Decision 2). Proceeding
  with (a): unchanged. Options:
  - (a) Leave `claimable` (`cmd/wf/main.go:275-291`) as it is. Nothing
    offers or claims the step, and step 2 says no claim.
  - (b) Refuse it ("confirmed by `wf done`, no claim (FLOW-03)"). This makes
    the README's "takes no claim or budget" mechanical, at the cost of one
    more branch and test, and it is not needed for this incident.

  Recommended: (a). Choose (b) if you want the README line enforced.
- **a4. An attended `wf next -feature F` on a merge-waiting item**
  (Decision 1). Proceeding with (a). Options:
  - (a) `none` and the merge-wait reason, like any blocked item. The
    attended tick runs step 2 first anyway, so a merged PR has already moved
    to Close.
  - (b) Offer the merge step to `NextFor` only, so an attended run can
    confirm it. This duplicates step 2, and an attended run would get a step
    with no dispatch row.

  Recommended: (a).
- **a5. FLOW-05's wording** (Decision 3). Proceeding with (a). Options:
  - (a) Add "or the human's merge (FLOW-08)" to the blocking list at
    `process.md:193-197`.
  - (b) No rule text. The class-1 clause already implies it, and only code
    changes. The rule would still not say that a merge-waiting item yields
    within its own class, which is the gap the incident exposed.

  Recommended: (a).
- **a6. QUEUE.md's "Next steps" column.** Proceeding with (a): unchanged.
  `writeActive` lists `x3-merge (cloud)` for a merge-waiting item
  (`summary.go:141-171`), and **Needs Jeff** already says "PR #n waiting for
  your merge" (`summary.go:119-121`). Options:
  - (a) Leave it. The summary lists ready steps and does not claim `wf next`
    offers them.
  - (b) Show "merge (Jeff)" in that column. This changes the golden file for
    a cosmetic gain.

  Recommended: (a).

## Design

Queue (`tools/workflow/queue`):

```text
IsMerge(st):  stepTemplates(st.ID) ok, a template step, name "merge",
              and (feature, phase 7) or (process, phase 3); any -r<n>
ready(f, worker):
  for each pending, Offered, needs-done step st:
    IsMerge(st)            -> wait "the human's merge of <prRef(f)> (FLOW-08)"
    affinity mismatch      -> wait "<affinity> step <id>"   (as today)
    leased to another      -> wait                           (as today)
    cloud and cloudHold    -> wait                           (as today)
    otherwise              -> return st
Next, NextFor: unchanged; a merge-waiting item is reported blocked and the
               walk goes on to the next item in Ordered()
```

`IsMerge` sits in `phases.go` beside `IsGate` and `IsDesignReview`, which
use the same parse. Its condition matches `waitsOnMerge`
(`summary.go:88-92`) on the step instead of the item.

Prototype check on a scratch copy of `beb51ba`, with only the `ready()`
change and `IsMerge`. Replaying the workflow branch:

- `9649de9` (the incident): design-principles and rule-running-line-match
  are reported `waiting on the human's merge of PR #104/#105 (FLOW-08)`, and
  `next hyperv-relay-per-build-channel f1-design-r3 cloud`.
- `f177cdd`: `next build-link-subnet f1-design-r2`. This is what the
  2026-11-04 tick did by hand.
- `cb67d90`: `next security-suite f1-design`.
- `go test ./queue/` fails only the two assertions named in the Problem
  section.

Skill and docs:

- `.claude/skills/workflow-tick/SKILL.md` step 2 (`:61-65`): add "`wf next`
  never offers these steps (FLOW-05); this is where they complete."
- `docs/development/process.md` FLOW-05, as in Decision 3.
- Workflow branch, at x4-close: `README.md:100` adds "never offered by
  `wf next`", and the `wf next` paragraph (`:148-152`) adds "skips an item
  waiting on the human's merge". These are bookkeeping on the workflow
  branch, as #103's x4-close did.
- `docs/book/src/concepts/overview.md` has no queue pseudocode, so FLOW-15
  has nothing to update there. `docs/book/src/guide/` does not document
  `wf` (UX-01 does not apply).

**Before the merge.** Nothing on this branch reaches the running loop until
Jeff merges it (FLOW-14). Until then, this item's own `x3-merge` will hit
the same bug. While it waits, it heads class 2 and starves the classes
below. Merging it soon after its PR goes green limits that to one or two
ticks. Sessions in that window keep stepping past by judgment, as on
2026-11-04. x4-close's digest entry tells each machine to rebuild `dist/wf`
from the merged `main`.

## Defaults and overrides

None. No flag, config key or default is added or changed. Skipping a merge
step is not a fallback. It is the only path, for every worker, with no
override: no worker is ever offered a merge step. The merge is still
confirmed only by `wf done` in the tick's step 2, as today.

## Cross-driver impact

None. No driver or product code changes.

## Security note

`security_boundary: none`. No build-environment boundary changes, and no
new input. The change only removes steps from what `wf next` offers.

## Validation plan

- **Tier 0, `tools/workflow/queue`:**
  - New `TestNextSkipsMergeWait`:
    - The incident's shape: a process item at `x3-merge` (class 2, holds
      nothing) and an in-progress fix in phase 1 with a ready cloud
      `f1-design-r3`. `Next("cloud")` returns the fix. `blocked` names the
      process item with `waiting on the human's merge of PR #104 (FLOW-08)`.
    - A feature at `f7-merge` (class 4, in progress, priority 1) ahead of an
      in-progress class-4 feature with a ready cloud step (priority 2): the
      second is returned.
    - The same, with only a queued class-4 feature behind: it starts
      (`Starts`), within the in-progress limit.
    - An `-r2` merge step (`f7-merge-r2`) is skipped too.
    - A host worker and a human worker get the merge reason, not
      `cloud step x3-merge`.
    - With no PR recorded, the reason says `no PR recorded`.
    - `NextFor` on the merge-waiting item returns no pick and the reason.
  - New `TestIsMerge`: `f7-merge`, `f7-merge-r2` and `x3-merge` are merge
    steps. `f6-merge-main`, `x2-implement`, an added `f7-other` and
    `m6-merge-round` are not.
  - Rework `TestNextTakesImplementationFirst` (`queue_test.go:217-219`):
    once the class-1 feature blocks, `Next` returns no pick, and `blocked`
    holds the merge reason.
  - Rework `TestPRSlotIsSerial` (`queue_test.go:570-572`): with a at
    `f7-merge` holding the slot, b reports the PR-slot reason and a the
    merge reason, and there is no pick. The rest of the test is unchanged.
  - `queue_test.go:198-203` (a fix starts ahead of a phase-7 merge check)
    and the hosts tests pass unchanged.
- **`tools/workflow/cmd/wf`** has no `_test.go`. Its `next` printing is
  unchanged.
- **Check in use, at x2-implement:** the branch's `wf next -worker cloud/x`
  on scratch copies of the workflow branch at `9649de9`, `f177cdd` and
  `cb67d90` gives the three picks listed in Design. The digest records them.
- `make test` and `make lint` (FLOW-17). No build-tagged files are touched.
- **No native validation.**

## Implementation plan

To be confirmed at x2-implement, after the review (process item, FLOW-03),
one commit each, each green:

1. `tools/workflow/queue/phases.go` (FLOW-03, GLOBAL-01): `IsMerge`, with
   its doc comment, beside `IsDesignReview`; `TestIsMerge`.
2. `tools/workflow/queue/next.go` (FLOW-05, FLOW-08; Decision 1): `ready()`
   skips a merge step for every worker with the merge-wait reason. The doc
   comments on `ready` and `Next` (`:79-88`) say so. Tests:
   `TestNextSkipsMergeWait`, and the rework of
   `TestNextTakesImplementationFirst` and `TestPRSlotIsSerial` (TEST-01,
   TEST-02).
3. `docs/development/process.md` (FLOW-05; Decision 3): the blocking-list
   sentence.
4. `.claude/skills/workflow-tick/SKILL.md` (FLOW-14; Decision 2): step 2's
   clause.
5. Archive this design with its summary and open the PR (x2-implement,
   FLOW-12).
6. x4-close, on the workflow branch: the README lines in Design, and the
   digest note to rebuild `dist/wf`.

## Design review

## Consolidation

## Rollback notes

## Archive summary
