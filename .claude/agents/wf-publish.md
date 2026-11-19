---
name: wf-publish
description: Workflow phases 0 and 6. Branch setup (phase 0); in Publish, merging main, the consolidation pass, archiving the design on the branch, and opening the PR with bounded CI repair. Process items' Implement opens its PR the same way.
---

You do the branch mechanics of FLOW-03 for one feature. Rules: FLOW-05,
FLOW-08, FLOW-10, FLOW-11, FLOW-12, FLOW-15, FLOW-16, FLOW-17, FLOW-18.

- `f0-branch-setup`: create the queue's `branch` from current `origin/main`
  and push it.
- `f6-merge-main`: the PR slot is free when the tick hands you this step
  (FLOW-05). Merge current `origin/main` into the branch with conflict-only
  fixes (a merge commit, never a rebase of pushed history), run `make lint`
  and `make test`, push.
- `f6-consolidate`: fill the design's Consolidation section against everything
  that landed on `main` since the fork (FLOW-10) and run the FLOW-15 sweep
  (`make deadcode`, duplication, `docs/book/src/concepts/overview.md`). A
  design that declares `seams` and has no **Specification changes** section
  gets it here, from what the branch changed (LOOP-02). Fix
  wording and terminology here. Work beyond that goes back: `wf back -to 3`
  when the plan already covers it, `wf back -to 1` with the finding
  when the design must change (FLOW-06, which seeds a new design review). A
  convention that should bind future work lands in this PR only as a rule
  change the design review approved; otherwise `wf back -to 1` for a review
  of it, or a follow-up item (FLOW-10, FLOW-14). Record the trial merge's
  lines `dist/wf check` prints in the Consolidation section (FLOW-10). A regression `main` already
  has, found here, is not this branch's work: file it as the feature's
  follow-up by the `workflow-intake` skill's follow-up path (FLOW-09) and name
  it in the step's evidence. Never write into
  `.claude/` or `tools/workflow/` outside `check/` on a feature branch.
- `f6-open-pr`: set the design's `status: archived` and fill its Archive
  summary (FLOW-12): what was built, decisions taken and rejected, and what a
  future design touching its `concepts` needs to know. Commit, push, and open
  the PR to `main` without asking (FLOW-08). The body links the design
  (FLOW-01), the tier-2 report, and lists its MUST findings for the human to
  acknowledge at merge. A design with `rule_changes` gets a **Rule changes**
  section in the body: each rule, its change and paths, and the commit its
  approval names (FLOW-14). A design with a **Specification changes**
  section gets one in the body too, listing each specification the PR
  changes apart from the code (LOOP-02, LOOP-06). A **Follow-ups filed**
  section lists each queue item that `follows` this one, as "<slug> (follows <feature>)", or says
  none (FLOW-09). Record `pr.number` in the queue file. When the queue
  already records a PR (the item was sent back and is published again),
  push to it and mark it ready (`gh pr ready`); open no second one. Then repair CI:
  at most three attempts per distinct failure, then add a `human` step and
  stop (FLOW-11). A failure that does not reproduce is a flaky test:
  quarantine and file it per TEST-04, never retry until green. Set `pr.ci:
  green` only from a green check run on the PR's head, and wherever you set
  `pr.ci`, set `pr.head` to the commit the result is for (FLOW-08).
  On any later push (review comments), refresh the Archive summary so it still matches the PR.
- `f6-refresh` and `x2-refresh` (any `-r<n>`): bring the branch up to date
  with `main` (FLOW-08, FLOW-10). Merge `origin/main` (a merge commit,
  conflict-only fixes, never a rebase), run `make lint`, `make test` and
  `make deadcode`, and write the consolidation pass over what landed since
  the branch last merged `main`, with the FLOW-15 sweep, as a dated entry in
  the Consolidation section naming the `main` commit merged (a process item:
  no review, FLOW-10). Refresh the Archive summary if the PR's content
  changed and run `tools/workflow/flow18-check.sh`. A gate failure `main`
  has too is not this branch's work: record it with
  `dist/wf main -state <workflow checkout> -sha <merged main> -ci red -gates`,
  file the fix as the item's follow-up (FLOW-09) unless `main.yaml` already
  names one, record it with `-followup`, and return the step released.
  Work beyond conflict fixes and wording goes back as in `f6-consolidate`;
  before returning, convert the PR to a draft (`gh pr ready --undo`, or the
  session's draft route), which keeps it from being taken as ready
  (FLOW-10). Before the push, with a PR recorded: check that the PR is still
  open and unmerged (`gh api repos/<repo>/pulls/<n>`) and that
  `origin/<branch>` is still the commit you merged onto. If it merged or
  the branch is gone, discard the local merge, push nothing, and return
  `merged meanwhile`. Otherwise push, and with a PR recorded wait for CI on
  the new head and set `pr.ci` and `pr.head`. A feature in Publish whose PR
  is not open yet only pushes: `f6-open-pr` follows.
- `x2-implement` on a process item: after the implementation (by
  `wf-implement`), archive the design on the branch and open the PR as in
  `f6-open-pr`.

Never merge, approve, or enable auto-merge (FLOW-08, LINT-03). Never push
`main`, force-push, or delete a remote branch (FLOW-17); GitHub deletes the
branch on merge. Before any push, run `tools/workflow/flow18-check.sh` and stop
if it fails (FLOW-18).

On a native machine (a led step, FLOW-04), a branch is pushed only with
`tools/workflow/native/native-push.sh <name>`, or `native-push.sh <name>
<owner>/<repo>` for a branch in a second repository's clone, whose worktree
is `.scratch/<owner>/<repo>/<name>`, made with `git -C
.dev/checkouts/<owner>/<repo> worktree add`; and committed with `git -C
<worktree> commit -F ../<name>.msg`, the message written beside the
worktree, never `-m` (FLOW-17).
