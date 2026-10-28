---
name: workflow-tick
description: One run of BuildWarden's unattended feature loop (FLOW-13). Takes cloud steps from the workflow-branch queue in FLOW-05 order, dispatches each to its phase agent, writes state after every step, and writes the run's digest. Run by the daily scheduled task, or by hand as "/workflow-tick attended <feature>".
---

# Workflow tick

One run of the loop in FLOW-13. The rules are in `docs/development/process.md`
(FLOW-03..FLOW-18) and `docs/development/behavioral-linter.md`; this skill is
the procedure, not the rules (FLOW-14). The queue schema and claiming protocol
are in the `workflow` branch's `README.md`.

Arguments: none for the unattended run. `attended <feature>` means a human is
watching: it skips the priority-confirmation gate and takes only that
feature's cloud steps.

## Never

- Merge, approve, or enable auto-merge on any PR (FLOW-08, LINT-03).
- Push `main`, force-push, rewrite pushed history, or delete a remote branch
  (FLOW-16, FLOW-17). Open a PR only for an `m6-publish` whose
  `m6-confirm-pr` a human has done (FLOW-08).
- Edit `docs/development/`, `.claude/`, or `tools/workflow/` unattended
  (FLOW-14). Propose changes as a question for the human instead.
- Put secrets in prompts (FLOW-18).

## 1. Set up

From the `buildwarden/buildwarden` checkout:

```bash
tools/workflow/flow18-check.sh
```

If it fails, stop: push nothing, and report the failure as the run's only
output (FLOW-18). Then:

```bash
command -v entire && entire status
(cd tools/workflow && go build -o ../../dist/wf ./cmd/wf)
git fetch origin main workflow
git worktree add -B workflow .dev/workflow origin/workflow 2>/dev/null ||
  git -C .dev/workflow reset --hard origin/workflow
```

If `entire` is missing, carry on but make "sessions not recorded (FLOW-18)"
the digest's first line. Set `W=cloud/<UTC timestamp>` as this run's worker.

Read `.dev/workflow/config.yaml`. Unattended and `priority_confirmed: false`:
write the digest (step 4) saying the loop waits on the human's confirmation of
the priority order, push it, and stop.

## 2. Take steps

Repeat while this run has taken fewer than `run_budget.steps` steps:

1. `dist/wf next -state .dev/workflow -worker $W`. `none` ends the loop; keep
   the `blocked` lines for the digest. Attended: if the pick is not the named
   feature, end the loop.
2. Claim it: `dist/wf claim -state .dev/workflow -worker $W -feature F -step S`,
   adding `-new-run` on this run's first claim of F. Commit on the workflow
   checkout (`F: claim S`) and `git push origin workflow`. A rejected push
   means the claim lost (FLOW-04): `git fetch origin workflow`,
   `git -C .dev/workflow reset --hard origin/workflow`, and go back to 1.
3. Check out the feature branch in the code checkout (`git switch <branch>`
   after a fetch; `p0-branch-setup` creates it). The tree must be clean.
4. Dispatch with the Agent tool, `subagent_type` by step name:

   | Step | Agent |
   |---|---|
   | `p0-branch-setup`, `m0-merge-main`, `p6-publish`, `m6-publish` | `wf-publish` |
   | `p1-design`, `m1-consolidate` | `wf-design` |
   | `*-plan` | `wf-plan` |
   | `*-implement` | `wf-implement` |
   | `*-review` | `wf-review` |
   | `*-guide` | `wf-guide` |
   | `m8-archive` | `wf-archive` |
   | `p6-merge-round` | none: go straight to 5 |

   Steps may carry an `-r<n>` suffix after a return to Design. Give the agent
   the feature name, step id, the queue file path, the code checkout path, and
   the workflow checkout path. In Review, a red tier 1 goes to `wf-implement`
   for the fix and back to `wf-review`, inside the same step.
5. Finish the step on the workflow checkout:
   - The agent returned `back-to-design`:
     `dist/wf back -state .dev/workflow -worker $W -feature F -why "<finding>"`.
   - Otherwise:
     `dist/wf done -state .dev/workflow -repo . -worker $W -feature F -step S -run`.
     It runs the phase's done-when check (FLOW-03). If it fails, run
     `dist/wf release ... -why "<failed checks>"`; the next run retries.
   - `dist/wf validate -state .dev/workflow` must pass; fix what the agent
     wrote into the queue file if it does not.
   - Commit (`F: <what happened>`) and push the workflow branch (FLOW-13:
     state after every step). A rejected push: fetch, re-apply the step's
     state change on top of `origin/workflow`, push again.
6. If the step raised a new blocking question, notify the human now (the
   PushNotification tool when available), naming the feature and question.

## 3. Close

`tools/workflow/flow18-check.sh` again. Switch the code checkout back to
`main`. Leave no uncommitted work on any branch.

## 4. Digest

Write `.dev/workflow/digest/<YYYY-MM-DD>.md`, the date in America/Los_Angeles.
If the file exists, append a new `## Run <W>` section. Sections, in order:

- **Moved**: each step taken, its outcome, and the branch head it left.
- **Blocked on Jeff**: open blocking questions (options and recommendation),
  assumptions awaiting confirmation, `confirm-pr`, `sign-off`, `cleanup` and
  `escalate` steps, runaway budgets (FLOW-13), priority confirmation.
- **Waiting on a host**: `host:<name>` steps and open native gaps, by host.
- **Spent**: steps this run against `run_budget.steps`, and each touched
  feature's spent against estimate, in steps and runs.
- **Notes**: FLOW-18 check result, whether Entire recorded the session.

Commit and push. End the session with the digest's text as the final message,
so the scheduled task's notification carries it.
