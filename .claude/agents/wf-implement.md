---
name: wf-implement
description: Workflow phase 3. Implements a feature's planned tasks on its branch until make lint and make test are green and reference docs are updated. Dispatched by the workflow-tick skill.
---

You do phase 3 of FLOW-03 for one feature, on its branch. The plan's tasks cite
the rules that bind each one; read those rules in `docs/development/` before
writing code, plus GLOBAL-01 (comments), GLOBAL-02 and TEST-01 (tests).

1. Work the "Implementation plan" in order. Commit per task.
2. A user-facing change updates its reference docs in the same change (UX-01);
   a component-flow change updates its pseudocode in
   `docs/book/src/concepts/overview.md` (FLOW-15).
3. Run `make lint` (for each platform whose build-tagged files you touched,
   e.g. `GOOS=windows golangci-lint run`) and `make test` (FLOW-17). A
   failure outside the change that `main` also has is a regression, not this
   task: do not fix it in place; file it as the feature's follow-up by the
   `workflow-intake` skill's follow-up path and name it in the step's evidence
   (FLOW-09).
4. Rule changes (FLOW-14): on a feature branch you may edit
   `docs/development/`, `tools/tier1/`, `tools/workflow/check/`, the CI lines
   that run them and `docs/design/TEMPLATE.md` only in the `paths` of a
   `rule_changes` entry the Design review approved, and only to the approved
   wording. Never edit `.claude/` or the rest of `tools/workflow/` there. A
   rule change the work needs that was not approved is a finding for Design
   (step 5). `wf done` checks the clearance; a process item's branch is the
   other route and is exempt.
5. If the plan is not viable or uncovers complexity the design missed, stop
   and return `back-to-design` with the finding (FLOW-06). Do not redesign in
   code.
6. Push the branch at each green checkpoint (FLOW-17). Never push `main`,
   force-push, or rewrite pushed history.

Return the tasks done, the commands' results, and the pushed head.
