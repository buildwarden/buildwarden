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
  (`make deadcode`, duplication, `docs/book/src/concepts/overview.md`). Fix
  wording and terminology here. Work beyond that goes back: `wf back -to 3`
  when the approved plan already covers it, `wf back -to 1` with the finding
  when the plan must change (FLOW-06, which seeds a new design review). A
  convention that should bind future work is proposed as a rule change for
  the design review or as a follow-up item, never written into
  `docs/development/` or `.claude/` on a feature branch (FLOW-14).
- `f6-open-pr`: set the design's `status: archived` and fill its Archive
  summary (FLOW-12): what was built, decisions taken and rejected, and what a
  future design touching its `concepts` needs to know. Commit, push, and open
  the PR to `main` without asking (FLOW-08). The body links the design
  (FLOW-01), the tier-2 report, and lists its MUST findings for the human to
  acknowledge at merge. Record `pr.number` in the queue file. Then repair CI:
  at most three attempts per distinct failure, then add a `human` step and
  stop (FLOW-11). A failure that does not reproduce is a flaky test:
  quarantine and file it per TEST-04, never retry until green. Set `pr.ci:
  green` only from a green check run on the PR's head. On any later push
  (review comments), refresh the Archive summary so it still matches the PR.
- `x2-implement` on a process item: after the implementation (by
  `wf-implement`), archive the design on the branch and open the PR as in
  `f6-open-pr`.

Never merge, approve, or enable auto-merge (FLOW-08, LINT-03). Never push
`main`, force-push, or delete a remote branch (FLOW-17); GitHub deletes the
branch on merge. Before any push, run `tools/workflow/flow18-check.sh` and stop
if it fails (FLOW-18).
