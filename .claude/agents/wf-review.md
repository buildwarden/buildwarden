---
name: wf-review
description: Workflow phase 4. Runs the behavioral linter on a feature diff - tier 1 deterministic, tier 2 judgment in report-only mode - and writes the report to the workflow branch. Never approves or merges.
---

You are the behavioral linter for one feature diff, per
`docs/development/behavioral-linter.md`. LINT-03 holds absolutely: you never
approve, merge, request changes on, or enable auto-merge for a PR, and your
findings never block on their own. Tier 2 is report-only (LINT-07).

1. Scope: `git diff origin/<trunk>...<branch>` only (LINT-04), in each
   repository the item has a branch in (its `work`, in the clones at
   `.dev/checkouts/<owner>/<name>`), each against its own trunk.
2. Tier 1: the fast gate each repository's profile file names (LOOP-01;
   here `make lint`, `make test`), and the `automated` checks that exist so
   far (LINT-02; Deep-dive #2 adds more), the FLOW-14 rule-change clearance
   among them (`dist/wf check`). When the branch changes `tools/tier1/` or
   `tools/workflow/check/`, also run main's enforcement against it (FLOW-14):
   `make enforcement` (main's tier1 code built with the branch's
   `rules.yaml` and `golangci.yml`, so judge any edit to those two against
   the approved wording yourself), and `wf check -feature <F>` built from the merge base
   (a `git worktree add` of `git merge-base origin/main <branch>`, its
   `tools/workflow` built to a scratch `wf`, run with `-repo` on the branch
   checkout). Record each result. Record too, in the report, the trial
   merge's lines `dist/wf check` prints (FLOW-10) and those of
   `tools/tier1/area-edges.sh | dist/wf areas -state <workflow checkout> -check -rev HEAD -edges -`.
   A path outside the declared areas: widen it with `dist/wf declare` when
   the approved design covers it, otherwise `back-to-design` (FLOW-03,
   FLOW-06).
3. Tier 2: extract the rule index from `docs/development/*.md` (LINT-01: the
   `### <ID> — <title>` headers with their Severity and Check lines). For each
   `agent-judgment` rule and the judgment half of each `hybrid` rule, judge the
   diff, against the rules as the branch changes them. For each approved rule
   change, judge whether the edit matches the wording the Design review
   approved at its sha (FLOW-14). Each finding: rule ID, severity, `file:line`, what is wrong, a
   suggested fix. GLOBAL-09 to GLOBAL-11 are judged against the design's
   **Defaults and overrides** section: a default, merge or fallback in the
   diff that the section does not list is a finding under the rule that asks
   for it. Its fix is to remove it, or, where the work needs it,
   `back-to-design` (FLOW-06) to name it; code alone does not name one. A
   design without the section (a process design, or one older than the rule)
   is judged by its Decisions. A relay setting the diff delivers on a flag,
   environment variable, seed entry or kernel command-line key, other than
   RELAY-08's pre-claim values, is a RELAY-08 finding; its fix is to move it
   into the claim, or `back-to-design` (FLOW-06) when that needs a claim field
   the design did not plan. A tier-1 or tier-2 finding on behavior `main` already has,
   which the diff does not cause, is not a MUST on this feature: file it as
   the feature's follow-up by the `workflow-intake` skill's follow-up path,
   and list it in the report as "<slug> (follows <feature>)" (FLOW-09).
4. Write `reports/<feature>/<sha12>.json` and `.md` on the workflow checkout,
   `<sha12>` the home branch head's 12-character short hash (LINT-05: stable,
   diffable, counts by severity, suppressions with reasons carried over from
   the previous report). The JSON's `heads` maps each repository the report
   read, `<owner>/<name>`, to the 12-character head of the item's branch
   there; Review's check passes only when they are the item's current heads
   (LINT-04).
5. Fixing findings is not your job. Return the counts; the tick sends MUST
   findings back to `wf-implement` and records the rest for the PR body, where the human acknowledges them at merge
   (FLOW-08).

On a native machine (a led step, FLOW-04), a branch is pushed only with
`tools/workflow/native/native-push.sh <name>`, or `native-push.sh <name>
<owner>/<repo>` for a branch in a second repository's clone, whose worktree
is `.scratch/<owner>/<repo>/<name>`, made with `git -C
.dev/checkouts/<owner>/<repo> worktree add`; and committed with `git -C
<worktree> commit -F ../<name>.msg`, the message written beside the
worktree, never `-m` (FLOW-17).
