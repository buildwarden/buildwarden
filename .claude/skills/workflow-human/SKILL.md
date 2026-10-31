---
name: workflow-human
description: An attended session with Jeff that drains the workflow queue's human steps - blocking questions, assumption confirmations, sign-off, cleanup, escalations and the priority order. Use when Jeff asks what the loop needs from him.
---

# Human session

The `human` worker of FLOW-04. Rules: FLOW-07 (questions), FLOW-08 (sign-off),
FLOW-11 and FLOW-13 (escalations), FLOW-16 (cleanup); this skill is the
procedure (FLOW-14). The human decides; you present and record.

## Set up

Same as `workflow-tick` step 1 (the FLOW-18 check, build `dist/wf`, check out
`.dev/workflow`). Worker: `W=human/jeff`. Read the latest file in
`.dev/workflow/digest/`.

## Work through, in this order

0. **Process items** (`kind: process`, FLOW-03) before anything else: their
   `x1-design` .. `x4-archive` steps are all Jeff's, and while one is in
   progress it holds the workers its `holds` name (FLOW-05). Show each with
   its phase, holds and branch; record each step with `dist/wf claim` and
   `dist/wf done` as `human/jeff`.
1. **Priority order.** If `config.yaml` has `priority_confirmed: false`, show
   `dist/wf status` and the proposed order with its reasons, ask Jeff to
   confirm or reorder, renumber the `priority` fields as he says, and set
   `priority_confirmed: true`.
2. **Blocking questions**, highest-priority feature first. Ask each with
   AskUserQuestion: its options, the recommended one first. Record the answer
   with `dist/wf answer -worker $W -feature F -question Q -answer "..."`,
   which also completes the matching `-answer-<Q>` step.
3. **Assumptions**, batched per feature: confirm each the same way, or turn a
   rejected one into a blocking question for Design.
4. **`sign-off` steps.** Show the PR, the tier-2 report with its MUST findings
   to acknowledge or dismiss with a recorded reason, any unconfirmed
   assumption, and the security note if `security_boundary` is set (FLOW-08,
   LINT-03). Jeff merges the PR himself; you never do. Once it is merged,
   `dist/wf done` the step: its check confirms the merge.
5. **`cleanup`, `escalate` and runaway steps.** Show the commands or the
   decision needed; record the outcome in the queue history; raise an
   estimate only on Jeff's word.

Commit and push the workflow branch after each decision. Finish with a short
summary of what was decided and what the next tick will pick up.
