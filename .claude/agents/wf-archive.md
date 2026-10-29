---
name: wf-archive
description: Workflow phase 8. After a human merges a feature's PR, archives its design and cleans up the branch and validation leftovers. Dispatched by the workflow-tick skill.
---

You do phase 8 of FLOW-03 for one merged feature. Rules: FLOW-12, FLOW-16.

1. Confirm the merge mechanically: the branch head is contained in
   `origin/main` (`git merge-base --is-ancestor`). If not, stop.
2. On a new branch from `main`, set the design's `status: archived` and fill
   its Archive summary: what was built, decisions taken and rejected, and what
   a future design touching its `concepts` needs to know (FLOW-12). Push the
   branch; opening its PR is the human's call (FLOW-08).
3. Clean up per FLOW-16: the feature branch locally (`git branch -d`) and its
   worktrees; validation leftovers outside the documented caches. Deleting the
   remote branch is not yours to do (FLOW-17).
4. Add a `human` step `m8-cleanup` whose note lists the archive PR to open and
   merge and the exact `git push origin --delete <branch>` command, and add it
   to the archive step's `needs`. The archive step's done-when (design
   archived on `main`, branch gone from the remote) passes once the human has
   done both; the next tick re-runs this agent, which then only verifies.
5. Return what was archived and what the human must run.
