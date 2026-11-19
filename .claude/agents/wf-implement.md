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
3. Run the fast gate the repository's profile file names (LOOP-01; here
   `make lint`, for each platform whose build-tagged files you touched,
   e.g. `GOOS=windows golangci-lint run`, and `make test`; FLOW-17), in each
   repository whose branch the item changes (its `work`, in the clone at
   `.dev/checkouts/<owner>/<name>`; no fast gate where the repository has no
   profile), and push each branch there at its green checkpoints. A
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
5. A path the branch changes outside the areas the design declares
   (FLOW-03): when the approved design covers it, widen the declaration with
   `dist/wf declare`; otherwise return `back-to-design` with the path
   (FLOW-06).
6. If the plan is not viable or uncovers complexity the design missed, a
   fallback the design's **Defaults and overrides** does not name among them
   (GLOBAL-10), stop and return `back-to-design` with the finding (FLOW-06). Do not redesign in
   code.
7. Push the branch at each green checkpoint (FLOW-17). Never push `main`,
   force-push, or rewrite pushed history.

Return the tasks done, the commands' results, and the pushed head.

On a native machine (a led step, FLOW-04), a branch is pushed only with
`tools/workflow/native/native-push.sh <name>`, or `native-push.sh <name>
<owner>/<repo>` for a branch in a second repository's clone, whose worktree
is `.scratch/<owner>/<repo>/<name>`, made with `git -C
.dev/checkouts/<owner>/<repo> worktree add`; and committed with `git -C
<worktree> commit -F ../<name>.msg`, the message written beside the
worktree, never `-m` (FLOW-17).
