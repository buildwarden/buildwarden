---
feature: workflow-streamline
status: draft
round: process
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
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
- **A changed plan gets a new review.** When a feature returns to Design
  (FLOW-06), from Implement, Review, or Publish's consolidation, the design
  review step is seeded again (`-r2`, ...). The review covers what changed
  and why.
- **"Trivial" is decided mechanically, not by the agent's judgment.** Design
  may mark the review `skip` only when all of these hold:
  `alters_relationships: false`, no `seams`, `security_boundary: none`, no
  blocking `native_validation`, no assumed questions, and the plan changes no
  rule. `wf check` enforces this, and the digest lists every skipped review so
  Jeff can call one back. Rejected: letting Design judge triviality freely.
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

- a1: proceeding with **one open feature PR at a time**. The alternative is
  every finished feature opening its PR, with each merge triggering a
  re-merge and re-consolidation of every other open PR. That means more CI
  runs and more re-review for Jeff. Confirm.
- a2: proceeding with **cloud Implement and Publish for process items after
  Jeff's design review,** with his merge as the sign-off. The alternative is
  to keep process items attended throughout. Confirm.
- a3: proceeding with **the skip criteria above** as the definition of a
  trivial design. Confirm, or tighten them, for example "no new exported API".
- a4: proceeding with **escalations as the only other human steps**: runaway
  budget (FLOW-13), CI repair exhausted (FLOW-11), a third return to Design
  (FLOW-06), and a remote branch that did not auto-delete. Blocking questions
  stay as they are (FLOW-07). Confirm.

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
| workflow-streamline | process/1 | This item: its own design review is the session that follows this draft. |

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

Pending. This draft goes to Jeff as the first design review session.

## Consolidation

Filled in at Publish.

## Rollback notes

## Archive summary

Filled in at Publish.
