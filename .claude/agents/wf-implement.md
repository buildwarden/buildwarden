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
   e.g. `GOOS=windows golangci-lint run`) and `make test` (FLOW-17).
4. If the plan is not viable or uncovers complexity the design missed, stop
   and return `back-to-design` with the finding (FLOW-06). Do not redesign in
   code.
5. Push the branch at each green checkpoint (FLOW-17). Never push `main`,
   force-push, or rewrite pushed history.

Return the tasks done, the commands' results, and the pushed head.
