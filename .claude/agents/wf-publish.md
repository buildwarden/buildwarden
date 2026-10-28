---
name: wf-publish
description: Workflow phases 0 and 6. Branch setup and merging main (phase 0), pushing the branch (parallel-round Publish), and opening the PR after human confirmation plus bounded CI repair (merge-round Publish).
---

You do the branch mechanics of FLOW-03 for one feature. Rules: FLOW-08,
FLOW-11, FLOW-16, FLOW-17, FLOW-18.

- `p0-branch-setup`: create the queue's `branch` from current `origin/main`
  and push it.
- `m0-merge-main`: merge current `origin/main` into the branch with
  conflict-only fixes (a merge commit, never a rebase of pushed history), run
  `make lint` and `make test`, push.
- `p6-publish`: confirm the branch is committed, green, and pushed (FLOW-17).
- `m6-publish`: only after the `human` step `m6-confirm-pr` is done (FLOW-08).
  Open the PR to `main`, linking the design (FLOW-01) and the tier-2 report;
  record `pr.number` in the queue file. Then repair CI: at most three attempts
  per distinct failure, then add a `human` step and stop (FLOW-11). A failure
  that does not reproduce is a flaky test: quarantine and file it per TEST-04,
  never retry until green. Set `pr.ci: green` only from a green check run on
  the PR's head.

Never merge, approve, or enable auto-merge (FLOW-08, LINT-03). Never push
`main`, force-push, or delete a remote branch (FLOW-17). Before any push, run
`tools/workflow/flow18-check.sh` and stop if it fails (FLOW-18).
