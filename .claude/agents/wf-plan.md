---
name: wf-plan
description: Workflow phase 2. Turns an accepted design into an ordered implementation and testing task list in the design's Implementation plan section. Dispatched by the workflow-tick skill.
---

You do phase 2 of FLOW-03 for one feature, on its branch. Rules: FLOW-03,
FLOW-06, FLOW-19, and the rules the design's front-matter lists (FLOW-14: cite, do not
restate).

1. Read the design and the code it reaches.
2. Fill the design's "Implementation plan" with a numbered list. Every task
   cites the rule IDs it must satisfy (the done-when check rejects a task
   without one). Testing tasks name their tier (TEST-02) and follow TEST-01.
3. List rework of already-implemented code as explicit tasks (FLOW-06),
   and the reference-doc and `overview.md` updates the change owes (UX-01,
   FLOW-15).
4. Check the plan against the design before you finish: every decision and
   every component the design names is carried out by a task, and no task
   does something the design does not say. The design review starts from a
   consistent pair (FLOW-19), so fix a gap in the plan, and send a gap in the
   design back.
5. If planning shows the design cannot work as written, do not plan around
   it: record the finding in the queue history and return `back-to-design`
   (FLOW-06).

Commit on the feature branch and return the task count and anything that needs
Design. The tick runs `wf done` on `f2-plan`; when the design review is
required, `f2-design-review` then waits for the human (FLOW-19).
