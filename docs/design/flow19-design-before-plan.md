---
feature: flow19-design-before-plan
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-05, FLOW-06, FLOW-07, FLOW-10, FLOW-14, FLOW-15, FLOW-17, FLOW-19,
  TEST-01, TEST-02]
rule_changes: []
concepts: [workflow, design-review, plan, queue, wf-check, process-item]
related: [design-approved-rule-changes.md, human-review-shows-design.md]
---

# The design review follows Design; Plan follows the approval

## Problem

FLOW-19 has the human review "a non-trivial design and its implementation
plan together" (`docs/development/process.md:530-531`), with "the plan
against the design" as one of the review's topics (`:537-538`). The queue
does the same: phase 2 seeds `f2-plan`, then `f2-design-review` needing it
(`tools/workflow/queue/phases.go:43`). So Jeff reviews a plan built on
assumptions he has not confirmed yet. When he changes one, the plan is
partly wrong and has to be redone.

build-link-subnet review r3 (2026-11-04) shows the cost and the fix. The
r3 entry is in `docs/design/build-link-subnet.md` on
`origin/feat/build-link-subnet`, and the attended-run section of
`.dev/workflow/digest/2026-11-04.md` records the same:

- Jeff reviewed the design at `c19e8f0` before the plan, as a trial of this
  change.
- Plan r3 had been written in parallel (`c21ba2a`), because the queue
  orders Plan first.
- a14 was answered (d), adding vm-mode DHCP for the direct-boot inits. Plan
  r3 had to be redone to fold it in (`c227ad0`, tasks 14-16 and 36-38).
- Planning then found no design change, so there was no second review.
- In the queue, `f2-plan-r3` was marked done after the review answers were
  in, and `f2-design-review-r3` after it
  (`.dev/workflow/queue/build-link-subnet.yaml`, history). That is the
  current order, worked around by hand.
- a14 (d) also left Decision 5's text out of date until Consolidation task
  56 (`a68ac2c`). The approval names `c19e8f0`, the design as read, so the
  approved sha is older than the design Plan worked from.
- The r3 addendum (2026-11-05) approved rule change BOOT-03 "at `303e713`"
  on its own line, after the approval and with no return to Design
  (`303e713`, `c6a9b51`, both after `5fc1f6d`). The route works, and the
  guard in Decision 4 has to allow it.

Jeff's rule (2026-11-05): make the trial permanent. The design review
follows Design directly. Plan comes after the approval. A feature returns
for a second review only if planning finds a reason to change the design.

## Prior art

- **FLOW-19** (`process.md:525-570`): the review and its record, the skip
  criteria, and the delta rule for a re-review (`:561-566`). This item
  changes when the review happens and what it covers. The criteria and the
  record are unchanged.
- **FLOW-06** (`:209-216`): a return to Design re-reviews "the changed
  plan". After this change, a return re-reviews the changed design.
- **FLOW-03** (`:109-110`): Plan's done-when contains "The design review is
  recorded as approved, or validly skipped". Process items already work the
  new way: `x1-design` is the design and its review (`:132`), and the plan
  is written after the review, at x2-implement
  (`design-approved-rule-changes.md`, Implementation plan: "Written at
  x2-implement, after the review").
- **FLOW-14 as amended by #96** (`:372-414`): this is a process item. It
  edits `docs/development/`, `.claude/` and `tools/workflow/` on its own
  branch after review, which is the first route. The `rule_changes` route is
  for features, and a process item's branch is exempt from the clearance
  (`:405-406`). So `rule_changes: []`, and the wording changes are in **Design**
  below. FLOW-14's own rule still holds after Plan: a rule change Plan finds
  is not cleared and goes back to Design (`:397-398`).
- **design-approved-rule-changes** Decision 7 already anticipated this
  trial: a rule change found in planning is one reason for a second review.
- **human-review-shows-design**: the review page shows the design as read.
  Once the plan is gone from the review, the page has no Implementation
  plan on a first review.

## Decisions

1. **The review moves to the front of phase 2 and keeps the id
   `f2-design-review`.** The phase-2 template becomes `design-review`
   (human), then `plan` (cloud) needing it. With `design_review: skip`, only
   `plan` is seeded, with no needs. The gate becomes the seeded template
   step of the phase that no other step of the same seed needs, instead of
   the last template step (`IsGate`, `phases.go:98-118`). For every phase
   today the two agree (phase 6: `open-pr`); for phase 2 it makes `plan` the
   gate under the new order and `design-review` the gate for an item seeded
   under the old order (Decision 8).
   - **Rejected: `f1-design-review` in phase 1.** `design_review` is decided
     inside phase 1, after the phase's steps are seeded (`Seed`,
     `phases.go:121`). A phase-1 review would need seeding on `f1-design`'s
     completion, a gate that is not a template step, and a phase-1 check
     that knows which step is completing. security-suite (phase 1) would
     also gain a step mid-phase.
   - **Rejected: a phase of its own.** It renumbers phases 3-8. That
     renames every in-flight `f3`-`f8` id, including build-link-subnet's
     `f6-*`, overflows `stepIDRe`'s single phase digit `[0-8]`
     (`phases.go:89`), and changes every rule that cites a phase number.
2. **Plan waits for the approval.** `f2-plan` needs `f2-design-review`, so
   Plan works from confirmed assumptions. FLOW-07's "confirmed before
   implementation starts" (`:232`) becomes "before planning starts".
   - **Rejected: Plan in parallel, with the approval checked at the gate.**
     r3 shows the cost: one answer redid six tasks.
3. **The review covers the design, not the plan.** FLOW-19's "the plan
   against the design" bullet is replaced by "the Validation plan and what
   Plan will work from". Plan checks itself against the approved design
   (wf-plan step 4, with a new reason: the design is approved, so a design
   gap goes back per FLOW-06). The automated part checks the approval;
   Review and the PR judge the plan's execution.
   - **Rejected: also show Jeff the plan for information.** That brings
     back the second sitting this change removes. The digest's Moved line
     gains Plan's task count (task 9), which is enough to call a review
     back.
4. **A design change found by Plan goes back to Design.** This covers
   anything Plan finds that is a reason to change the design: an
   unworkable decision, a gap, or a rule change (FLOW-14). Plan records the
   finding and returns `back-to-design` (FLOW-06). The fresh review is
   judged on its delta from the approval (FLOW-19, `:561-566`), so a
   minimal change can still skip. Otherwise there is no second review.
   Plan does not edit the design outside its Implementation plan.
   - **Mechanical guard.** At the Plan gate, `wf check` compares the
     design's impact fields (`impactDelta`, `review.go:137`) and its
     `rule_changes` with the design at the last `approved at <sha>`. Any
     difference fails the check and names it, so Plan cannot widen the
     design past its review. A `rule_changes` entry absent at that sha
     passes when it has its own `rule change <ID> approved at <sha>` line
     and is in the front-matter at that line's sha, as `cleared` already
     requires for FLOW-14 (`rulechange.go:166`). That keeps r3's addendum
     route: a rule change Jeff approves on its own line. A skipped review
     has no approval; it is rechecked by its skip criteria (`skipValid`),
     as today.
   - **On failure.** wf-plan runs `dist/wf check -feature F` before it
     finishes. A guard failure is a design change, so it returns
     `back-to-design` with the delta as the finding, instead of leaving
     `f2-plan` pending for the next tick to re-run against the same check.
   - **Rejected: judgment only** (wf-plan's instruction alone). The fields
     are cheap to compare, and FLOW-14's history shows that a prohibition
     written in agent files fails.
   - **Rejected: fail on any change outside the Implementation plan.**
     Wording fixes and Plan's own queue notes would bounce the feature for
     nothing.
5. **Changes Jeff makes in the sitting are folded into the design before
   the approval is recorded.** The `approved at <sha>` then names the
   design as approved, with the sitting's changes folded in, not the design
   as read. Plan follows it literally and the Decision 4 guard compares
   against it, so a fold that changes an impact field does not fail Plan.
   This avoids r3's out-of-date Decision 5. FLOW-19's definition of the sha
   and workflow-human 3.5's "of the design as read" change to match
   (Design). The review page stays the design as read; the entry records
   that sha as `read at <sha>` when it differs (`approvedRe` does not match
   it). An "approved with a change" rule-change line names the same
   fold-in commit, so the entry is in the front-matter at that sha.
   - **Rejected: note the change in the review entry and let Plan or
     Consolidation fix the text** (what r3 did, through task 56).
6. **`wf done` on a design-review step checks its approval.** The review
   is no longer its phase's gate, so marking it done ran no check. It now
   runs FLOW-19's `reviewed()` (`review.go:44`): an approval for every
   review seeded. The Plan gate still runs `reviewed()` as well
   (`check.go:47-49`).
7. **`Seed` picks a retry suffix free for every template step in the
   phase, not only the first** (`phases.go:126-129`). With `design-review`
   first, a phase-2 re-entry where no `f2-design-review` exists but
   `f2-plan` does would reuse the id `f2-plan`. That happens when the
   earlier review was skipped, or was dropped by `wf back`, as
   build-link-subnet's r1 and r2 reviews were. `Seed` also drops a `needs`
   entry whose template step was not seeded (a skipped review).
8. **No queue migration; old-order items finish under the new code.** No
   feature is in phase 2 today (`.dev/workflow/queue/*.yaml`: security-suite
   is in phase 1, build-link-subnet in phase 6, and the rest are queued,
   done, or process items). Templates apply only when a phase is seeded.
   But this item has no `holds` (FLOW-05), so security-suite can seed phase
   2 in the old order before the merge. The needs-based gate (Decision 1)
   handles that pair: `f2-design-review` needs `f2-plan`, so the review
   stays the gate, `wf done f2-plan` runs no check, and `wf done
   f2-design-review` runs the phase-2 check as before. Only phases seeded
   after the merge get the new order. build-link-subnet is untouched unless
   it returns to Design, and Decision 7 covers that. Process items are
   unchanged.
   - **Rejected: a hold on cloud steps until this merges.** It stops the
     whole loop for a case the gate rule covers.
   - **Rejected: a one-off `wf migrate` rewrite.** It has to run at the
     merge, and a pair whose Plan is already done would need its review
     moved behind it.
9. **"The approved plan" becomes "the plan", and "the plan must change"
   becomes "the design must change".** The human no longer approves the
   plan, so FLOW-06 (`:215`), FLOW-10 (`:317`), wf-publish (`:19`) and
   workflow-tick (`:109`) say "work the plan already covers". The return to
   Design is for a design change: FLOW-10 (`:318`) and wf-publish (`:20`)
   say "when the design must change". FLOW-06's opening keeps "the plan is
   no longer viable" for Implement's finding, since there is no return to
   Plan alone; the fresh review is judged on the design's delta (FLOW-19),
   so a plan-only return can skip it. The meaning of `wf back -to 3` is
   unchanged.

## Questions

### Blocking

None.

### Assumed

- **a1. Where the review sits** (Decision 1). Proceeding with
  `f2-design-review` first in phase 2, `f2-plan` needing it. Options:
  - (a) as stated;
  - (b) `f1-design-review` in phase 1, seeded when `f1-design` completes;
  - (c) a new phase between Design and Plan.

  Recommended: (a).
- **a2. Plan's start** (Decision 2). Proceeding with Plan waiting for the
  approval. Options:
  - (a) as stated;
  - (b) Plan may run in parallel, with the approval required at its gate;
  - (c) parallel only in an attended run.

  Recommended: (a).
- **a3. The guard on Plan** (Decision 4). Proceeding with the Plan gate
  failing on an impact-field or `rule_changes` difference from the last
  approval, a rule change with its own approval line excepted. Options:
  - (a) as stated;
  - (b) wf-plan's judgment only;
  - (c) as stated, and also any edit outside the Implementation plan.

  Recommended: (a).
- **a4. What Jeff sees of the plan** (Decision 3). Proceeding with nothing
  beyond Plan's task count on the digest's Moved line. Review and the PR
  judge the plan's
  execution. Options:
  - (a) as stated;
  - (b) the plan's task list in the digest, so he can call a review back;
  - (c) a short plan read-through added to the next review sitting.

  Recommended: (a).
- **a5. Changes made in the sitting** (Decision 5). Proceeding with folding
  them into the design text before `approved at <sha>`. Options:
  - (a) as stated;
  - (b) note them in the review entry, with Plan or Consolidation updating
    the text.

  Recommended: (a).

## Design

`docs/development/process.md`:

- **Shape, item 2** (`:20-22`): "a non-trivial design, after Design and
  before Plan, and again only when a later phase finds the design must
  change (FLOW-19)."
- **FLOW-03 Plan** (`:110`): "The design review recorded as approved, or
  validly skipped, before planning starts (FLOW-19). Implementation and
  testing tasks listed, each citing the rules it must satisfy; rework of
  existing code is listed as explicit tasks. The design's impact fields and
  `rule_changes` unchanged since its last approval, when there is one
  (FLOW-19); a skipped review is rechecked by its skip criteria."
- **FLOW-06** (`:209-216`): "When implementation finds that the plan is no
  longer viable, or implementation or planning finds that the design must
  change or uncovers complexity it did not consider". "The changed design
  goes through a design review again, judged on its delta (FLOW-19)"
  replaces "the changed plan". "Work the plan already covers" replaces "the
  approved plan".
- **FLOW-07** (`:232`): "before planning starts".
- **FLOW-10** (`:317-318`): "to Implement for work the plan already covers,
  or to Design (FLOW-06) when the design must change".
- **FLOW-19**:
  - Opening (`:530-531`): "After Design and before Plan, the human reviews
    a non-trivial design ...".
  - The plan bullet (`:537-538`) becomes "the Validation plan and what Plan
    will work from: native validation steps, tier-2 gaps, and, on a
    re-review, the finding and the tasks it touches".
  - The new rule: "Plan works from the approved design. A reason Plan finds
    to change it is a finding for Design (FLOW-06), and the fresh review
    follows the delta rule below. Otherwise there is no second review."
  - The record (`:545-547`): "for an approval, the commit of the design as
    approved, with any changes made in the sitting folded in (`approved at
    <sha>`)". An "approved with a change" rule-change line names that same
    commit.
  - Delta paragraph (`:561-562`): "a fresh review follows the new design".
  - Automated part (`:568-570`): add "a design-review step's completion and
    the Plan gate check its approval, and the Plan gate fails on an impact
    or `rule_changes` change since it". The agent part judges whether the
    session covered the design.

Queue (`tools/workflow/queue/phases.go`):

```text
phase 2: design-review (human)        -- seeded unless design_review: skip
         plan (cloud, needs design-review if seeded)   -- the gate
```

## Cross-driver impact

None. No driver or product code changes.

## Security note

`security_boundary: none`. No build-environment boundary changes. The human
gate moves earlier and is not removed. Plan cannot pass its gate without
the approval (`reviewed()`), and the new guard adds a check: Plan cannot
change the design's impact or rule changes past what was approved.

## Validation plan

- **Tier 0, `tools/workflow/queue`** (`queue_test.go`):
  - `TestCompleteWalksOneRound`: the order is `f2-design-review f2-plan`
    (`want`, `:175`). The review needs nothing; the plan needs the review
    and is the gate: the `gates` map (`:180`) names `f2-plan`, not
    `f2-design-review`, and the `f.Steps[3]` assertion (`:195`, "a human
    step after Plan") becomes `f.Steps[2]`, a human step before Plan.
  - New: an old-order phase 2 (`f2-plan`, then `f2-design-review` needing
    it) under the new code: `f2-plan` is not the gate, the review is.
  - `TestSkippedDesignReviewIsNotSeeded`: `f2-plan` has no needs.
  - New: a phase-2 re-entry with `f2-plan` present and no
    `f2-design-review` seeds `-r2` ids for both, and `Validate` finds no
    duplicate.
  - `TestBackToDesignEscalatesOnTheThirdReturn`: after each return,
    `f2-design-review-rN` precedes `f2-plan-rN`.
  - `summary_test.go` and `testdata/queue.md`: updated for the new needs.
- **Tier 0, `tools/workflow/check`** (`check_test.go`):
  - `check.Review` fails with no approval and passes with one. The
    `wf done` dispatch in `main.go` has no test (`tools/workflow/cmd/wf`
    has no `_test.go`); it is covered by "Check in use" below.
  - The Plan gate fails when `seams`, `alters_relationships`,
    `security_boundary`, blocking hosts or `rule_changes` differ from the
    design at the approval, and passes when they do not. A `rule_changes`
    entry added after the approval passes with its own approval line at a
    sha that has it, and fails without one.
  - A skipped review still goes through `skipValid`.
  - The process round's `x1-design` check is unchanged.
- **Check in use:** the first feature to seed phase 2 after the merge
  (security-suite, unless it seeds first and finishes in the old order):
  `wf done` on its review runs `check.Review`, and its Plan gate runs the
  guard.
- **No native validation.**

## Implementation plan

Written in full at x2-implement (process item, FLOW-03). The expected tasks,
one commit each:

1. `docs/development/process.md` (FLOW-03, FLOW-06, FLOW-07, FLOW-10,
   FLOW-19): the wording in Design.
2. `tools/workflow/queue/phases.go` (FLOW-03, FLOW-19; Decisions 1, 7):
   - the phase-2 template order and needs, and its comment (`:41-42`);
   - `IsGate` picks the seeded template step no step of the same seed
     needs;
   - `Seed`'s suffix chosen free for every template id, and needs on
     unseeded steps dropped;
   - the `Back` comment (`:153`) says "before Plan";
   - `queue.go`'s `DesignReview` comment (`:74-76`): it decides whether
     phase 2 seeds a design-review step before Plan.
3. `tools/workflow/check/review.go` and `check.go` (FLOW-19, FLOW-06;
   Decisions 4, 6):
   - `check.Review(env, f)` runs `reviewed()`;
   - the Plan gate adds the approval-delta check. It reuses `impactDelta`
     and adds a `rule_changes` comparison that accepts an entry with its
     own approval line, as `cleared` (`rulechange.go`) does;
   - `reviewed()`'s comment says it is the design-review step's and the
     Plan gate's check.
4. `tools/workflow/cmd/wf/main.go` (FLOW-03, FLOW-19; Decision 6): `done`
   on a `*-design-review` step runs `check.Review` (beside `:245`).
5. Tier-0 tests per the Validation plan (TEST-01, TEST-02).
6. `.claude/agents/wf-plan.md` (FLOW-06, FLOW-14, FLOW-19):
   - Plan starts from the approved design;
   - Plan edits only the Implementation plan;
   - step 4's reason (`:22-24`, "The design review starts from a consistent
     pair") becomes: the design is approved, so fix a gap in the plan and
     send a gap in the design back (FLOW-06);
   - Plan runs `dist/wf check -feature F` before finishing; a guard
     failure, or any other reason to change the design, a rule change
     without its own approval line included, returns `back-to-design` with
     the delta as the finding;
   - the closing line (`:30-31`): `f2-design-review` precedes `f2-plan`,
     and the tick's `wf done` on `f2-plan` is the phase gate.
7. `.claude/agents/wf-design.md` (FLOW-19): step 5 says a `required` review
   is seeded before Plan.
8. `.claude/skills/workflow-human/SKILL.md` step 3 (FLOW-19, FLOW-07;
   Decisions 3, 5):
   - read the design, and the plan only on a re-review that a Plan finding
     caused;
   - 3.2 becomes the Validation plan and what Plan will work from;
   - 3.5 folds in-sitting changes into the design before `approved at
     <sha>`, and "of the design as read" becomes "of the design as
     approved, changes folded in", with `read at <sha>` when it differs.
9. `.claude/skills/workflow-tick/SKILL.md` and `.claude/agents/wf-publish.md`
   (FLOW-06, FLOW-10; Decisions 3, 9):
   - "work the plan already covers" (workflow-tick `:109`, wf-publish
     `:19`);
   - "when the design must change" (wf-publish `:20`);
   - workflow-tick's Moved line (`:137`) gives Plan's task count for an
     `f2-plan` step.
10. `overview.md` (FLOW-15): no component flow changes; none expected.
11. Consolidation, `make lint`, `make test` (FLOW-10, FLOW-17).
12. At x4-close, on the `workflow` branch (FLOW-03): README's phase-2 row
    (`f2-design-review`, then `f2-plan`) and its return paragraph ("Design
    seeds a fresh review before Plan").

## Design review

- **r1, 2026-11-06: approved at `e9b8653`** (Jeff, x1-design, FLOW-19). He
  reviewed this design alongside flow19-design-before-plan,
  wf-later-phase-host-steps and budgets-active-only, all on one review page
  (published 2026-11-05 with every assumption first). He agreed with every
  recommendation: a1 to a5 are confirmed as drafted, option (a).

## Consolidation

Filled in at x2-implement (FLOW-10, FLOW-15). Known overlaps:

- wf-later-phase-host-steps (queued) also edits `Back` in `phases.go`.
- budgets-active-only (queued) may edit the queue code.

## Rollback notes

Revert the PR, then fix any phase-2 item seeded under the new order. The
old `IsGate` takes `design-review`, the last template step, as the gate,
and the old phase-2 check runs `plan()` first (`check.go:47-49`,
`:388-401`), so a new-order review that precedes its plan cannot be marked
done, and `f2-plan` waits on it. The revert is safe as is while no feature
has a pending new-order phase-2 pair. Otherwise, on the `workflow` branch,
edit `.dev/workflow/queue/<feature>.yaml` for each such pair:

- review pending: drop `f2-plan`'s need on the review and make the review
  need `f2-plan` (the old order), then `dist/wf validate`;
- review done, plan pending: no edit, but the old code runs no check on
  `wf done f2-plan`, so run `dist/wf check -feature <feature>` first and
  mark it done only when the phase-2 checks pass.

Commit and push the queue change with the revert.

## Archive summary

Filled in at Publish, in the feature's PR, and refreshed when the PR changes
(FLOW-12).
