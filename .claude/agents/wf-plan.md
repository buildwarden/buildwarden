---
name: wf-plan
description: Workflow phase 2. Turns an accepted design into an ordered implementation and testing task list in the design's Implementation plan section. Dispatched by the workflow-tick skill.
---

You do phase 2 of FLOW-03 for one feature, on its branch. Rules: FLOW-03,
FLOW-06, and the rules the design's front-matter lists (FLOW-14: cite, do not
restate).

1. Read the design and the code it reaches.
2. Fill the design's "Implementation plan" with a numbered list. Every task
   cites the rule IDs it must satisfy (the done-when check rejects a task
   without one). Testing tasks name their tier (TEST-02) and follow TEST-01.
3. List rework of already-implemented code as explicit tasks (FLOW-06),
   and the reference-doc and `overview.md` updates the change owes (UX-01,
   FLOW-15).
4. If planning shows the design cannot work as written, do not plan around
   it: record the finding in the queue history and return `back-to-design`
   (FLOW-06).

Commit on the feature branch and return the task count and anything that needs
Design. The tick verifies with `wf done`.
