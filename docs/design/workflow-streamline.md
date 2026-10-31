---
feature: workflow-streamline
status: archived
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07, FLOW-08, FLOW-10, FLOW-12, FLOW-14, FLOW-15, FLOW-16, FLOW-17, LINT-03]
concepts: [workflow, feature-queue, phases, merge-round, design-review, sign-off, archive, branch-cleanup, process-items]
related: [workflow-bootstrap.md, native-host-loop.md, native-loop-followups.md]
---

# Streamline the feature loop

## Problem

A feature needs too many steps, PRs and human gates to reach `main`. The two
features the loop has finished show it:

| | relay-leaf-crl | container-inputs |
|---|---|---|
| Steps | 20 | 22 + archive |
| PRs | 2 (#70, then archive #71) | 2 (#72, then archive #74) |
| Human steps | 5 (confirm-pr, sign-off, cleanup, two answers) | sign-off, cleanup, answers |
| Phases run twice | design, plan, implement, review, guide, publish | the same, plus a second parallel round |

The overhead comes from three places:

1. **A separate Archive phase after the merge.** `wf-archive` opens a second
   PR that sets `status: archived` and fills in the summary, then adds an
   `m8-cleanup` human step to merge that PR and delete the remote branch.
2. **The merge round repeats phases 0 to 6.** After the parallel round, a
   feature merges `main` and runs consolidate, plan, implement, review, guide
   and publish again. Usually the second pass finds nothing to change.
3. **Human steps that aren't decisions.** Sign-off is a queue step even though
   merging the PR already is the sign-off. Cleanup is a queue step even
   though GitHub can delete a merged head branch. Assumptions wait until
   sign-off to be confirmed.

There is also a missing gate. Jeff wants every non-trivial design reviewed with
him before implementation: a high-level pass over what changes and how, held as
one interactive session, not a PR or a list of questions. Today nothing gives
the design that kind of review. Plan follows Design automatically, and Jeff
first sees the whole picture at the PR.

## Prior art

- `workflow-bootstrap.md` set up the two-round model and the queue. Its
  reason for the merge round was the Windows-driver consolidation: a feature
  should be reconciled against everything that landed while it was being
  built (FLOW-10). That reason still holds, but a full second pass through
  every phase is the wrong tool for it. A consolidation step that sends the
  feature back only when it finds real work keeps the guarantee.
- FLOW-08 already says opening a PR needs no confirmation. relay-leaf-crl's
  `m6-confirm-pr` step predates that, and the tooling no longer seeds it.
- FLOW-06 already sends a feature back to Design when implementation outgrows
  the plan. That return is the trigger for a fresh design review.

## Decisions

- **One round.** Phases 0 to 8 run once. The parallel and merge rounds go
  away. Consolidation (FLOW-10) and the duplication, dead-code and overview
  sweep (FLOW-15) move into Publish, after `main` is merged. Rejected:
  keeping the merge round and skipping its unchanged phases, which still
  means a round transition, a merge queue step, and two sets of step ids.
- **A design review gate at the end of Plan,** for every non-trivial design
  (FLOW-19, new). It sits after Plan, not after Design, so Jeff reviews the
  design and the implementation plan together and can check they are
  consistent. Rejected: a gate after Design (the plan isn't written yet); a
  PR review (Jeff wants one interactive session).
- **The review is a session, not a step that waits on a PR.** The
  `workflow-human` skill runs it. The agent walks Jeff through a high-level
  overview, Jeff approves or asks for changes, and the outcome is recorded in
  the design's new **Design review** section and in the queue.
- **Assumptions are confirmed in the design review,** not at sign-off. That
  moves every intent question ahead of implementation (FLOW-07).
- **A changed plan gets a new review, unless the change is trivial against
  the approved design.** When a feature returns to Design (FLOW-06), from
  Implement, Review, or Publish's consolidation, the design review step is
  seeded again (`-r2`, ...) and covers what changed and why. Triviality for a
  re-review is judged on the delta from the **last approved design**, not on
  the design as a whole or on `main` (Jeff, at review r1): a minimal change
  doesn't need re-approval, even when the design itself needed review the
  first time.
- **"Trivial" is decided mechanically, not by the agent's judgment.** Design
  may mark the review `skip` only when all of these hold:
  `alters_relationships: false`, no `seams`, `security_boundary: none`, no
  blocking `native_validation`, no assumed questions, and the plan changes no
  rule. `wf check` enforces this, and the digest lists every skipped review so
  Jeff can call one back. Rejected: letting Design judge triviality freely.
  For a re-review, the same criteria apply to the delta from the design at
  the approved commit, which the Design review section records: the impact
  fields (`alters_relationships`, `seams`, `security_boundary`, blocking
  `native_validation`) are unchanged from the approved design, no question
  was added, and no rule was added to what the plan changes.
- **Merging is the sign-off.** No `sign-off` queue step for Jeff to claim.
  Phase 7 is `merge`: a cloud step that a tick completes once its check shows
  the branch head is in `origin/main`. Jeff still acknowledges or dismisses
  the tier-2 MUST findings (FLOW-08, LINT-03), as a PR comment or by merging.
  The PR body lists them.
- **The archive goes in the feature PR.** Publish sets `status: archived` and
  fills in the Archive summary in the same PR, and refreshes both on every
  push after review comments. "On merge" (FLOW-12) then holds exactly: the
  design is archived in the same commit that lands the code. Rejected: a
  separate archive PR (the overhead above).
- **Phase 8 becomes `close`, bookkeeping with no PR and no human step.** A
  tick confirms the merge, deletes its local branch with `git branch -d`,
  clears validation leftovers (FLOW-16), and marks the item done. GitHub's
  "Automatically delete head branches" setting removes the remote branch
  (Jeff turned it on 2026-10-31). The close check confirms that. If the
  branch is still there, an `escalate` step lists the command.
- **One open feature PR at a time** (assumed, a1). A feature finished through
  Guide waits for the PR slot. When the slot frees, it merges `main`,
  consolidates and opens its PR. A `main` that doesn't move under an open PR
  keeps consolidation correct without re-checking.
- **Process items implement unattended on their branch** (assumed, a2). Design
  is Jeff's: the design review session. Implement and Publish are cloud
  steps, and the merge is Jeff's sign-off, which FLOW-14 requires. A process
  branch changes nothing the running loop reads until it merges. FLOW-14's
  "not unattended" becomes "not on `main` without the human's merge".

## Questions

### Blocking

None. Jeff answered the gate question on 2026-10-31: non-trivial designs get
a last-pass overview with him, as one interactive session, also when
implementation changes the plan.

### Assumed

All confirmed by Jeff at review r1, with the approval.

- a1, **confirmed**: one open feature PR at a time. The alternative was every
  finished feature opening its PR, with each merge triggering a re-merge and
  re-consolidation of every other open PR: more CI runs and more re-review.
- a2, **confirmed**: cloud Implement and Publish for process items after
  Jeff's design review, with his merge as the sign-off. The alternative was
  keeping process items attended throughout.
- a3, **confirmed**: the skip criteria above define a trivial design, and a
  re-review applies them to the delta from the approved design.
- a4, **confirmed**: escalations are the only other human steps: runaway
  budget (FLOW-13), CI repair exhausted (FLOW-11), a third return to Design
  (FLOW-06), and a remote branch that did not auto-delete. Blocking questions
  stay as they are (FLOW-07).

## Design

### Phases (FLOW-03)

| # | Phase | Steps | Done when |
|---|---|---|---|
| 0 | Branch setup | `f0-branch-setup` (cloud) | Branch exists on the remote. |
| 1 | Design | `f1-design` (cloud), `f1-answer-<q>` (human) | Design complete per FLOW-02; no open blocking question; `design_review` set. |
| 2 | Plan | `f2-plan` (cloud), then `f2-design-review` (human), unless the review was skipped | Plan written; the review is recorded as approved, or validly skipped (FLOW-19). |
| 3 | Implement | `f3-implement` (cloud) | Tasks done; `make lint`, `make test` green; reference docs updated; pushed. |
| 4 | Review | `f4-review` (cloud) | Tier 1 green; tier-2 report written. |
| 5 | Guide | `f5-guide` (cloud) | Narrative docs updated. |
| 6 | Publish | `f6-merge-main`, `f6-consolidate`, `f6-open-pr` (cloud), `f6-validate-<host>` (host, if blocking) | `main` merged; Consolidation section filled (FLOW-10, FLOW-15); design `archived` with its summary **on the branch**; PR open, CI green; blocking native validation done. |
| 7 | Merge | `f7-merge` (cloud, completes by check) | Branch head contained in `origin/main`. Jeff merged it. |
| 8 | Close | `f8-close` (cloud) | Remote branch gone; local branch deleted; design archived on `main`; item `done`. |

`f6-merge-main` waits for the PR slot (a1): it is held while another feature
is in phases 6 or 7 with a PR open. If `f6-consolidate` finds work beyond
conflict fixes and wording, it sends the feature back: to Implement for code
the plan already covers, or to Design (FLOW-06) when the plan must change,
which seeds a new design review.

The human steps on a normal feature are `f1-answer-*` (only if Design raised
a blocking question), `f2-design-review` (only if non-trivial), and the merge
itself, which isn't a queue step. relay-leaf-crl would have been 10 steps
with 1 PR and 2 human touches.

### Process items

| # | Phase | Affinity | Done when |
|---|---|---|---|
| 1 | Design | human (the design review session) | Design on the branch, review recorded. |
| 2 | Implement | cloud (a2) | Pushed; `make lint`, `make test` green; design archived on the branch. |
| 3 | Merge | cloud, completes by check | Merged by Jeff. |
| 4 | Close | cloud | As phase 8. |

`holds` work as before (FLOW-05).

### The design review session (FLOW-19, new)

The `workflow-human` skill runs a design review as one sitting:

1. **Overview first.** What problem, what changes, and how, at component level:
   which components, seams and flows, before and after, with the `overview.md`
   pseudocode delta where a flow changes.
2. **The plan against the design.** The task sequence, what each task touches,
   and where the plan and the design disagree or leave gaps. This is the
   consistency check Jeff asked for.
3. **Risks and what's deferred:** native validation, tier-2 gaps, rules changed.
4. **Assumptions,** confirmed or rejected one by one (FLOW-07).
5. **Outcome:** `approved`, or `changes` with Jeff's notes, which sends the
   feature back to Design with the notes as the finding.

The agent may render the overview as a page when a diagram helps, but the
review happens in the session. The design's **Design review** section records
each review (`r1`, `r2`, ...): date, outcome, the decisions Jeff made, and
for a re-review, what changed since the last one.

Design front-matter gains:

```yaml
design_review: required   # required | skip (FLOW-19 criteria; wf check enforces skip)
```

Each approval in the Design review section names the commit the approved
design was read at (`approved at <sha>`). On a return to Design, `wf check`
compares the current design with the design at that commit to decide whether
`skip` is valid for the re-review.

### Tooling (`tools/workflow`)

- `queue/phases.go`: one `feature` round with the letter `f` and the phase
  table above; `process` as above; no round transition and no `merge-round`
  step. `Complete` moves `feature` phase 8 and `process` phase 4 to done.
  `Seed` adds `f2-design-review` unless the design says `skip`. `Back` already
  seeds fresh `-rN` ids, so a return to Design gets a new review for free.
- `queue/next.go`: the merge-queue hold becomes the PR-slot hold on
  `f6-merge-main`. A cloud worker may take `f7-merge` at any time; it
  completes only when its check passes, and otherwise stays pending without
  spending budget.
- `check/check.go`: phase 1 checks `design_review`; phase 2 checks the review
  is recorded or the skip is valid; phase 6 checks `status: archived` and a
  filled Archive summary on the **branch**, plus the PR; phase 7 checks the
  merge; phase 8 checks the remote branch is gone and the local branch is
  deleted.
- `wf migrate`: rewrites in-flight items once. A `parallel` item keeps its
  phase number under the `feature` round, and done steps keep their old ids.
  A `merge` item past phase 6 keeps its remaining steps under the old
  meaning. Old `p`/`m` ids stay valid in history and for `wf validate`.
- The workflow branch's `README.md`: schema for `round: feature`,
  `design_review`, and the new step names.

### Agents and skills (`.claude/`)

- `wf-design`: no merge-round consolidation pass. It sets `design_review`.
- `wf-plan`: ends by checking the plan against the design, so the review
  starts from a consistent pair.
- `wf-publish`: `f0-branch-setup`; `f6-merge-main`; `f6-consolidate` (FLOW-10,
  FLOW-15, the Consolidation section, back to Implement or Design when needed);
  `f6-open-pr` (archive the design on the branch, open the PR with the tier-2
  MUST findings listed, repair CI per FLOW-11, refresh the Archive summary on
  every later push).
- `wf-archive` is removed. `f8-close` is done inline by `workflow-tick`, with
  no PR and no remote deletion.
- `workflow-tick`: the new step names; `f7-merge` checked every run; the
  "Never" list updated: process branches may be edited unattended after their
  design review (a2), `main` never.
- `workflow-human`: a **Design review** section (above) replaces the sign-off
  and cleanup sections. Escalations stay.

### Rules (`docs/development/`)

- `process.md`: "The shape of the process" rewritten for one round; FLOW-03
  (both phase tables); FLOW-04 (human steps: answers, design review,
  escalations); FLOW-05 ("merge queue" becomes "PR slot"); FLOW-06 (a return to
  Design reseeds the review); FLOW-07 (assumptions confirmed at design
  review); FLOW-08 (the merge is the sign-off, tier-2 acknowledged on the PR);
  FLOW-10 and FLOW-15 (consolidation and the sweep in Publish); FLOW-12 (the
  archive goes in the feature PR); FLOW-14 (process branches, a2); FLOW-16
  (Close; remote branch auto-deleted, escalate otherwise); new **FLOW-19 —
  Non-trivial designs get a design review**.
- `behavioral-linter.md`: "sign-off" means acknowledgement on the PR.
- `docs/design/TEMPLATE.md`: drop `round`; add `design_review`; rename
  "Consolidation (merge round)" to "Consolidation"; add a "Design review"
  section; the Archive summary is "filled in at Publish".

## Cross-driver impact

None. This changes the development process only, no driver code (`drivers: []`).

## Validation plan

- Tier 0: `tools/workflow` unit tests for the new phase table, `Seed` with
  and without the review, `Back` reseeding `f2-design-review-r2`, the PR-slot
  hold, the check for each phase (including an invalid `skip`), and
  `wf migrate` on copies of the current queue files.
- Dry run: `wf migrate` on a copy of the `workflow` branch, then `wf validate`
  and `wf status`, compared by hand against the table in Migration below.
- First live use: the next feature through the loop (tier1-static-checks or
  design-front-matter-retrofit), watched in the digest.

## Migration

| Item | Now | After |
|---|---|---|
| container-inputs | merge/8, archive PR #74 open | Finishes under the old steps: merge #74, then the tick closes it. |
| design-front-matter-retrofit | parallel/4 review | feature/4 |
| tier1-static-checks | parallel/3 implement | feature/3. Its design was never reviewed, so a design review step is added before Implement continues, unless Jeff waives it. |
| queued features | parallel/0 | feature/0 |
| native-allowlist-scratch | process/1 | process/1, new phase table |
| workflow-streamline | process/2 | This item, reviewed and approved at r1. |

## Implementation plan

1. Rules: `process.md` (all the FLOW changes above, new FLOW-19),
   `behavioral-linter.md`, `TEMPLATE.md` (FLOW-14: rules first, tooling
   follows).
2. `tools/workflow/queue`: phase tables, `Seed`, `Complete`, the PR-slot hold;
   tests (TEST-01).
3. `tools/workflow/check`: per-phase checks above, including the skip
   criteria; tests.
4. `wf migrate` plus a test on fixtures copied from today's queue.
5. Agents and skills: `wf-design`, `wf-plan`, `wf-publish`, remove
   `wf-archive`, `workflow-tick`, `workflow-human`; `AGENTS.md` if a row
   changes.
6. Workflow branch: README schema, then `wf migrate` once this PR merges.
7. `make lint`, `make test`; archive this design in the same PR (the new
   FLOW-12).

## Design review

- **r1, 2026-10-31: approved** (Jeff, attended session), approved at
  `796bba2` plus the r1 caveat below. a1 to a4 confirmed with the approval.
  - Caveat, folded in without another review: when a feature returns to
    Design after consolidation finds breaking changes, triviality is judged
    against the previously approved design, not against `main`'s current
    design, so minimal changes don't trigger re-approval.
  - Jeff also asked for these reviews to be one interactive session with him,
    not a PR thread or a list of questions. That is FLOW-19's form.

## Consolidation

`main` merged at `34264d9` in `d1278df`, without conflicts.

- **Landed since the fork:** #74, the container-inputs archive (the last
  separate archive PR), and #73, qemu attaching the Windows prepare seed as
  removable USB. Neither touches `docs/development/`, `.claude/` or
  `tools/workflow/`.
- **Terminology:** "merge round", "parallel round", "sign-off step" and
  "Archive phase" are gone from the rules, agents and skills. "Sign-off" now
  means the human's merge (FLOW-08). The new terms are the PR slot, the design
  review, Merge and Close.
- **Abstractions:** one path per job. Consolidation and the FLOW-15 sweep live
  only in `f6-consolidate`; archiving lives only in `f6-open-pr` (and its
  process-item twin in `x2-implement`); `wf-archive` is removed.
- **Rules proposed:** FLOW-19 is new, and FLOW-03 to FLOW-08, FLOW-10, FLOW-12
  and FLOW-14 to FLOW-16 are amended, all in this PR.
- **Sweep:** `tools/workflow` has no dead code left from the rounds: the
  merge-round transition, the merge-queue hold and the `m`/`p` templates are
  removed. The `p`/`m` letters stay valid in step ids only for finished items'
  history. `overview.md` describes BuildWarden's components, not the
  development loop, so it is unchanged.
- **Left for Close, after the merge:** update the `workflow` branch's
  `README.md` schema (round `feature`, `design_review`, the new step names),
  then run `wf migrate` and push the migrated queue. Until then, the old `wf`
  on `main` and the queue agree; the new `wf` refuses the queue until it is
  migrated.

## Rollback notes

## Archive summary

Implemented 2026-10-31 in one attended session at priority 1, right after the
container-inputs archive (#74) showed the overhead once more.

- **Built.** One `feature` round (step ids `f<phase>-<name>`): Plan seeds a
  human `f2-design-review` unless Design records a valid `design_review: skip`;
  Publish is `f6-merge-main` (behind a one-PR slot), `f6-consolidate` and
  `f6-open-pr` (archives the design on the branch and opens the PR); `f7-merge`
  completes by check once the human merges; `f8-close` is inline bookkeeping.
  Process items: human design and review, then cloud implement and PR, merge,
  close. `wf back -to 3` returns consolidation work to Implement; `wf migrate`
  moves in-flight items. FLOW-19 checks: the recorded decision, the skip
  criteria, a re-review judged against the design at its last `approved at
  <sha>`, and one approval per seeded review. The merge check also works
  after GitHub deletes the branch, from the PR's merge commit.
- **Decisions taken:**
  - one round, with consolidation in Publish sending the feature back only
    when it finds real work;
  - a design review after Plan, so the design and the plan are reviewed
    together, held as one interactive session, not a PR thread (Jeff);
  - a re-review is judged on the delta from the last approved design, so a
    minimal change needs no re-approval (Jeff, at r1);
  - skip criteria checked mechanically, with skipped reviews listed in the
    digest;
  - the human's merge is the sign-off, with tier-2 MUST findings listed in the
    PR body;
  - the archive ships in the feature PR; GitHub's auto-delete removes the
    branch;
  - one open feature PR at a time (a1); process items implemented by the loop
    on their branch after review (a2); escalations as the only other human
    steps (a4).
- **Rejected:** keeping the merge round and skipping unchanged phases; a
  separate archive PR; a gate after Design (no plan to check yet); review on a
  PR; letting Design judge triviality freely; every finished feature opening
  its PR at once.
- **Validated.** Tier 0 tests for the phase walk, the skipped review, the PR
  slot, both kinds of `back`, each phase's checks (including an invalid skip
  and the re-review delta) and `wf migrate`; a dry run of `wf migrate` on a
  copy of the live queue, which validated and picked the right next step.
- **For future designs touching `workflow`, `phases`, `design-review` or
  `sign-off`:** the human's three touchpoints (questions, the design review,
  the merge) are the design. A new human step needs a reason it can't be one
  of those three or an escalation. A change to what makes a design trivial
  changes `check/review.go` and FLOW-19 together.
