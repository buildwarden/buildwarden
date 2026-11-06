---
name: wf-plan
description: Workflow phase 2. Turns an approved design into an ordered implementation and testing task list in the design's Implementation plan section. Dispatched by the workflow-tick skill.
---

You do phase 2 of FLOW-03 for one feature, on its branch. Rules: FLOW-03,
FLOW-06, FLOW-19, and the rules the design's front-matter lists (FLOW-14: cite, do not
restate).

1. Read the design and the code it reaches. Plan starts from the design as
   its review approved it (the last `approved at <sha>`), or as Design left
   it when the review was validly skipped (FLOW-19). Edit only the design's
   Implementation plan section.
2. Fill the design's "Implementation plan" with a numbered list. Every task
   cites the rule IDs it must satisfy (the done-when check rejects a task
   without one). Testing tasks name their tier (TEST-02) and follow TEST-01.
3. List rework of already-implemented code as explicit tasks (FLOW-06),
   and the reference-doc and `overview.md` updates the change owes (UX-01,
   FLOW-15). Each rule change the Design review approved (`rule change <ID>
   approved at <sha>`) gets its own task citing the rule and naming its
   `paths` (FLOW-14). A rule change the plan needs that the review did not
   approve sends the feature back to Design (FLOW-06, FLOW-14).
4. Check the plan against the design before you finish: every decision and
   every component the design names is carried out by a task, and no task
   does something the design does not say. The design is approved, so fix a
   gap in the plan, and send a gap in the design back (FLOW-06).
5. Run `dist/wf check -feature F` before you finish. It fails when the
   design's impact fields or `rule_changes` differ from the design at its
   last approval (FLOW-19). That failure, or any other reason to change the
   design (a decision that cannot work as written, a gap, a rule change
   without its own `rule change <ID> approved at <sha>` line), is a design
   change: do not plan around it or edit the design. Record the finding,
   with the delta the check names, in the queue history and return
   `back-to-design` (FLOW-06).

Commit on the feature branch and return the task count and anything that needs
Design. When the review is required, `f2-design-review` precedes `f2-plan` and
is done before Plan is dispatched; the tick's `wf done` on `f2-plan` is the
phase's gate (FLOW-03, FLOW-19).
