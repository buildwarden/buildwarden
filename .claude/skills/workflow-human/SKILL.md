---
name: workflow-human
description: An attended session with Jeff that drains the workflow queue's human steps - blocking questions, design reviews (one interactive session per non-trivial design), escalations, open PRs to merge, and the priority order. Use when Jeff asks what the loop needs from him, or to review a design.
---

# Human session

The `human` worker of FLOW-04. Rules: FLOW-07 (questions), FLOW-19 (design
review), FLOW-08 (the merge), FLOW-11, FLOW-13 and FLOW-16 (escalations); this
skill is the procedure (FLOW-14). The human decides; you present and record.

## Set up

Same as `workflow-tick` step 1 (the FLOW-18 check, build `dist/wf`, check out
`.dev/workflow`). Worker: `W=human/jeff`. Read the latest file in
`.dev/workflow/digest/`.

## Work through, in this order

0. **Process items** (`kind: process`, FLOW-03) before anything else. Their
   design is Jeff's: hold its design review (item 3 below) on the branch the
   queue names, and record the design step with `dist/wf claim` and
   `dist/wf done` as `human/jeff`. After that the loop implements the item on
   its branch; while one is in progress it holds the workers its `holds` name
   (FLOW-05). Show each with its phase, holds and branch.
1. **Priority order.** If `config.yaml` has `priority_confirmed: false`, show
   `dist/wf status` and the proposed order with its reasons, ask Jeff to
   confirm or reorder, renumber the `priority` fields as he says, and set
   `priority_confirmed: true`.
2. **Blocking questions**, highest-priority feature first. Ask each with
   AskUserQuestion: its options, the recommended one first. Record the answer
   with `dist/wf answer -worker $W -feature F -question Q -answer "..."`,
   which also completes the matching `-answer-<Q>` step.
3. **Design reviews** (`*-design-review` steps, FLOW-19), highest priority
   first, one feature per sitting. Check out the feature branch and read the
   design, its Implementation plan, and, for a re-review, the design at the
   last `approved at <sha>` and the finding that sent it back. Then walk Jeff
   through it, in this order, and let him steer:
   1. **Overview.** The problem, then what changes and how at component level:
      the components, seams and flows touched, before and after, and the
      `overview.md` pseudocode change where a flow changes. Render it as an
      artifact page when a diagram makes it clearer; the review itself stays
      in this session.
   2. **The plan against the design.** The task sequence and what each task
      touches; say plainly where they disagree or leave a gap.
   3. **Risks and what is deferred:** native validation, `tier2_gaps`, rules
      changed, a Bootstrap comparison or Security note to accept.
   4. **Assumptions**, each confirmed with `dist/wf answer`, or turned into a
      change.
   5. **Outcome.** Approved: add an entry to the design's Design review
      section (date, outcome, decisions, `approved at <sha>` of the design as
      read, and for a re-review what changed), commit and push the branch,
      then `dist/wf done` the step. Small changes Jeff makes while approving
      are folded in and noted in the entry, with no second review. Changes:
      record his notes in the entry, then
      `dist/wf back -worker $W -feature F -why "<his notes>"`.
4. **Escalations** (`escalate` steps, a runaway budget, CI repair exhausted, a
   branch that did not delete on merge): show the decision or the command
   needed; record the outcome in the queue history; raise an estimate only on
   Jeff's word.
5. **Open PRs**: list the features in phase 7 with their PR and the tier-2
   MUST findings it lists. Jeff merges them himself; you never do (FLOW-08,
   LINT-03), and the next tick sees the merge. A finding he dismisses is
   recorded as a PR comment with his reason.

Commit and push the workflow branch after each decision. Finish with a short
summary of what was decided and what the next tick will pick up.
