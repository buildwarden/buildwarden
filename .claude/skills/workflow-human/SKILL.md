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
`.dev/workflow`). Worker: `W=human/jeff`. A merge step stays `cloud` and is
never claimed, but this session confirms a merge with `dist/wf done` (FLOW-04):
one Jeff makes in the sitting, and any `merged:` line `dist/wf stale` prints
(item 5).
Read the latest file in `.dev/workflow/digest/`.

## Never

- Merge, approve, or enable auto-merge on any PR (FLOW-08, LINT-03).
- Change a trunk's protection: on GitHub, its branch protection rule and
  the rulesets that apply to it (FLOW-08).
- Change `.dev/workflow/areas.yaml`, mark an item urgent, narrow an item's
  areas or relate two items, except on Jeff's word as below (FLOW-05).

## Work through, in this order

0. **Process items** (`kind: process`, FLOW-03) before anything else. Their
   design is Jeff's: hold its design review (item 3 below) on the branch the
   queue names, and record the design step with `dist/wf claim` and
   `dist/wf done` as `human/jeff`, both at the outcome (3.5). After that the loop implements the item on
   its branch; while one is in progress it holds the workers its `holds` name
   (FLOW-05). A process design fills **Defaults and overrides** like a
   feature's (GLOBAL-09 to GLOBAL-11); `wf check` does not check it. Show each
   with its phase, holds and branch.
1. **Priority order.** If `config.yaml` has `priority_confirmed: false`, show
   `dist/wf status` and the proposed order with its reasons, ask Jeff to
   confirm or reorder, move each item he places with `dist/wf move
   -feature F -to N -why "<his reason>"` (wf renumbers the rest), and set
   `priority_confirmed: true`. `wf status` lists FLOW-05's class order: in
   implementation, process items, fixes (`follows: <item>`), new
   functionality. A position moves an item within its class only; an item
   moves across classes only by Jeff's word on its `follows` or `kind`,
   recorded in its history.
2. **Blocking questions**, in FLOW-05's order (`dist/wf status`). Ask each with
   AskUserQuestion: its options, the recommended one first. Record the answer
   with `dist/wf answer -worker $W -feature F -question Q -answer "..."`,
   which also completes the matching `-answer-<Q>` step.
3. **Design reviews** (`*-design-review` steps, FLOW-19), in FLOW-05's
   order, capped by load. An *ask* is one AskUserQuestion entry Jeff
   answers; the cap counts asks, not calls. Start the next review only while
   it would keep the sitting at or under 30 asks. A large design (more than
   4 assumed questions, or more than 4 `rule_changes` entries) is reviewed
   alone. At 30 asks, say so ("30 asks: stop here?") and stop unless Jeff
   says go on; his word to go on also holds a second review after a large
   one. An intake Jeff asks for mid-review is noted and done after that
   review's outcome. The review comes after Design and before Plan. Note the
   walkthrough's start time (UTC). Claim the review step (`x1-design` for a
   process item) only at the outcome, together with its done: a claim of a
   queued process item starts it, which would hold its `holds` and its
   areas through a review that may end in changes. Check out the feature branch and read the design, and, for a
   re-review, the design at the newest review entry's `approved at <sha>` and the finding that
   sent it back, with the Implementation plan only when a later phase's
   finding caused the re-review. Put the design
   in front of Jeff before the walkthrough (FLOW-19). If it has more than 4
   assumed questions, publish it as read as a private artifact page (the
   Artifact tool), with the commit SHA in its subtitle, give him the link, and
   republish if he changes the design during the sitting. Otherwise, or if
   publishing is not available, paste its Questions section in full with a
   short summary of the Decisions they rest on. Then walk Jeff through it, in
   this order, and let him steer:
   1. **Overview.** The problem, then what changes and how at component level:
      the components, seams and flows touched, before and after, and the
      `overview.md` pseudocode change where a flow changes. Where a diagram
      makes it clearer, add it to the design's page (publishing one if there
      is none); the review itself stays in this session. Confirm with Jeff
      the repositories the design names under **Design**, or that it names
      none and so works only here (LOOP-04). Show the repositories and
      areas it declares, the open items it is near or collides with and why
      (`dist/wf relations -state .dev/workflow -feature F`), and the lines of
      `tools/tier1/area-edges.sh | dist/wf areas -state .dev/workflow -check -edges -`
      (FLOW-05, FLOW-19). Jeff may narrow or widen the areas, name a
      collision the map does not show, or have the map changed (below).
      Where the design has
      **Specification changes**, go over it: each seam's specification and
      the requirements that change, or that the seam has none and what
      defines it (LOOP-02).
   2. **The Validation plan and what Plan will work from:** native
      validation steps (Design adds them; Plan only refines their notes),
      `tier2_gaps`, and, on a re-review, the finding, the tasks it touches,
      and how Design reconciled the later-phase steps `wf back` kept.
   3. **Risks and what is deferred:** native validation, `tier2_gaps`, rules
      changed, a Bootstrap comparison or Security note to accept, and the
      **Defaults and overrides** section, each named fallback in particular
      (GLOBAL-09 to GLOBAL-11).
      Ask the `rule_changes` entries (FLOW-14), each its own
      AskUserQuestion entry with its current and proposed wording, up to 4
      entries per call: approve, approve with a change, or reject. Record
      each approval in the Outcome entry as `rule change <ID> approved at
      <sha>`; a rejection is a change unless the design drops it. Refuse a
      feature's entry that names a LOOP, FLOW or LINT rule, whatever its
      paths: a loop rule changes only through a process item (LOOP-09).
   4. **Assumptions** (FLOW-07, FLOW-19). First show a table of every
      assumed question with its recommendation, marking the flagged ones
      (`Flagged: <reason>` in the design; you may add a flag, never remove
      one). Then ask one entry: "All unflagged as recommended", "Name
      exceptions" (free text), or "One by one". Each exception and each
      flagged question is then its own entry, up to 4 per call; under "One
      by one", every assumed question is, up to 4 per call. Each entry gives
      the question in full, each option with its consequence, the
      recommendation first, and the decision it rests on. Record each answer
      with `dist/wf answer`, one per question (an answer from the batch as
      `as recommended (batch), Jeff, <date>`), or turn it into a change.
   5. **Outcome.** Approved: first fold the small changes Jeff makes while
      approving into the design text and commit them, with no second review;
      Plan follows the design literally. Then add an entry to the design's
      Design review section, one top-level bullet led by `r<N>, <date>:
      <outcome>`, whose head for an approval reads `approved at <sha>`; an
      earlier approval it cites comes after that (date, outcome, decisions, `approved at <sha>` of
      the design as approved, changes folded in, `read at <sha>` of the
      design as read when it differs, what was folded in, and for a re-review
      what changed, and one line `sitting: <start>-<end> UTC, asks: <n>
      (<m> in batches)`, where an ask is one entry Jeff answered). A rule
      change "approved with a change" names the same fold-in commit. When
      the design says a process item needs work on the `workflow` branch
      after its merge, record that checklist as its state step:
      `dist/wf state-step -state .dev/workflow -worker $W -feature F -note
      "<the checklist>"`; `wf` adds it as `x4-state` when the item enters
      Close, and `x4-close` waits for it (FLOW-03). Commit and push the branch, then `dist/wf done` the
      step, which checks the approval. Changes: record his notes in the
      entry, then `dist/wf back -worker $W -feature F -why "<his notes>"`.
4. **Escalations** (`escalate` steps, `f8-escalate-returned` (a PR Jeff
   merged while its item was sent back; file its finding as a follow-up or
   drop it, on his word), a red `main` (FLOW-09), holders over a limit
   (FLOW-05), a runaway budget on an open item
   (FLOW-13), CI repair exhausted, a branch that did not delete on merge),
   and a tracked item's steps (FLOW-03, FLOW-04):
   show the decision or the command needed; record the outcome in the queue
   history; raise an estimate only on Jeff's word.
5. **Open PRs**: record `main`'s CI and its ruleset as `workflow-tick`
   step 2 does (`dist/wf main`), then run
   `dist/wf stale -state .dev/workflow -repo . -checkouts .dev/checkouts -worker $W`, which skips a PR
   Jeff has already merged. List the PRs that are ready to merge apart from
   those that wait for a refresh (FLOW-08, FLOW-10), each with the tier-2
   MUST findings it lists, and offer only the ready ones for merging. Jeff
   merges them himself; you never do (FLOW-08, LINT-03). After each merge he
   makes in the session, run `dist/wf main` and `dist/wf stale` again, then
   confirm it: `dist/wf done -state .dev/workflow -repo . -checkouts
   .dev/checkouts -worker $W -feature F -step <x3-merge|f7-merge|f6-merged-n>`,
   and `dist/wf commit -state .dev/workflow -worker $W -m "F: merge confirmed"
   queue/F.yaml`. Confirm each `merged:` line `wf stale` prints the
   same way. A failed check leaves the step pending; say so and go on. Then
   take the human steps the merge unblocked (a seeded `x4-state`); `x4-close`
   stays the cloud's, for the tick. Ask which PR he wants next;
   refresh that one with `/workflow-tick attended <item>`, wait for CI on the
   new head, and list it as ready. A finding he dismisses is recorded as a
   PR comment with his reason.
6. **Retrospective**, only when Jeff asks for one, by name or by asking how
   done items went (FLOW-13): run `dist/wf status -all` and read the done
   items' queue history, digests and archived designs. Otherwise leave done
   items out.

**On Jeff's word only** (FLOW-05):

- *A change to the area map*, here or at any other time: write a candidate
  file, show him
  `dist/wf validate -state .dev/workflow -areas <candidate> -repo .`,
  re-declare on his word any item it would put outside its areas, then save
  it as `.dev/workflow/areas.yaml` and push it with
  `dist/wf commit -worker human/<name>`.
- *Narrowing an item's areas* (`dist/wf declare`), and *a collision* the map
  does not show (`dist/wf relate -feature F -with G`, `-clear` to remove it).
- *The urgent mark* (`dist/wf urgent -feature F`, `-clear` to remove it):
  only for a security fix, judged as `workflow-intake` judges it, and
  declined for anything else, saying that the mark is for a security fix and
  that the limit is his to raise.

A lead is routing, not placement: any worker may set or clear one, and Jeff
may change one back with
`dist/wf lead -state .dev/workflow -worker human/<name> -feature F -host <name>`
(`-clear` to remove it, FLOW-04, FLOW-09). Each change is a history line,
and the run that made it lists it under **Moved**.

Push the workflow branch after each decision with
`dist/wf commit -state .dev/workflow -worker $W -m "<what was decided>" <paths>`
(`workflow-tick` step 2 gives its exit codes). Finish with a short
summary of what was decided and what the next tick will pick up.
