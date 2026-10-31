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
  (FLOW-16, FLOW-17). Opening a PR needs no confirmation (FLOW-08):
  `f6-open-pr` and a process item's `x2-implement` open theirs.
- Edit `docs/development/`, `.claude/`, or `tools/workflow/` anywhere but on
  a process item's branch after its design review (FLOW-14). On a feature
  branch, propose the change in the design for its review instead.
- Put secrets in prompts (FLOW-18).

## 1. Set up

From the `buildwarden/buildwarden` checkout:

```bash
tools/workflow/flow18-check.sh
```

If it fails, stop: push nothing, and report the failure as the run's only
output (FLOW-18). Then:

```bash
command -v entire && entire doctor --force && entire status
(cd tools/workflow && go build -o ../../dist/wf ./cmd/wf)
git fetch origin main workflow
git worktree add -B workflow .dev/workflow origin/workflow 2>/dev/null ||
  git -C .dev/workflow reset --hard origin/workflow
```

`entire doctor --force` installs Entire's git hooks, which a fresh clone lacks;
`entire status` must name `buildwarden/buildwarden-sessions` as the
checkpoint remote. If `entire` is missing, carry on but make "sessions not recorded (FLOW-18)"
the digest's first line. Set `W=cloud/<UTC timestamp>` as this run's worker.

Read `.dev/workflow/config.yaml`. Unattended and `priority_confirmed: false`:
write the digest (step 4) saying the loop waits on the human's confirmation of
the priority order, push it, and stop.

## 2. Take steps

First, confirm merges, which spend no budget and need no claim: for each
feature in phase 7 (`f7-merge`) and each process item in phase 3 (`x3-merge`),
run `dist/wf done -state .dev/workflow -repo . -worker $W -feature F -step S`.
It passes once the human has merged the PR (FLOW-08) and moves the item to
Close; otherwise leave it pending. Commit and push any that moved.

Then repeat while this run has taken fewer than `run_budget.steps` steps:

1. `dist/wf next -state .dev/workflow -worker $W` (attended: add
   `-feature <feature>`). `none` ends the loop; keep the `blocked` lines for
   the digest.
2. Claim it: `dist/wf claim -state .dev/workflow -worker $W -feature F -step S`,
   adding `-new-run` on this run's first claim of F. Commit on the workflow
   checkout (`F: claim S`) and `git push origin workflow`. A rejected push
   means the claim lost (FLOW-04): `git fetch origin workflow`,
   `git -C .dev/workflow reset --hard origin/workflow`, and go back to 1.
3. Check out the feature branch in the code checkout (`git switch <branch>`
   after a fetch; `f0-branch-setup` creates it). The tree must be clean.
   Close is the exception: it works on `main`.
4. Dispatch with the Agent tool, `subagent_type` by step name:

   | Step | Agent |
   |---|---|
   | `f0-branch-setup`, `f6-merge-main`, `f6-consolidate`, `f6-open-pr` | `wf-publish` |
   | `f1-design` | `wf-design` |
   | `*-plan` | `wf-plan` |
   | `f3-implement`, `f3-implement-r<n>` | `wf-implement` |
   | `x2-implement` | `wf-implement`, then `wf-publish` to archive and open the PR |
   | `*-review` | `wf-review` |
   | `*-guide` | `wf-guide` |
   | `f8-close`, `x4-close` | none: do it inline, below |

   Close (FLOW-16) takes no PR and no human step: confirm the branch head is
   in `origin/main`, remove any worktree on the branch, `git branch -d` it
   (never `-D`), clear the feature's validation leftovers outside the
   documented caches, then go to 5. If the remote branch is still there,
   release the step and add a `human` step `f8-escalate-branch` whose note
   gives the exact `git push origin --delete <branch>` command, and make
   `f8-close` need it.

   Steps may carry an `-r<n>` suffix after a return to an earlier phase. Give the agent
   the feature name, step id, the queue file path, the code checkout path, and
   the workflow checkout path. In Review, a red tier 1 goes to `wf-implement`
   for the fix and back to `wf-review`, inside the same step.
5. Finish the step on the workflow checkout:
   - The agent returned `back-to-design`:
     `dist/wf back -state .dev/workflow -worker $W -feature F -why "<finding>"`.
     Consolidation returning work the approved plan covers adds `-to 3`
     (back to Implement, FLOW-10).
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

- **Process items** first, when any is queued or in progress (FLOW-03,
  FLOW-05): each one's phase and what it holds (`dist/wf status` prints
  `process item, holds: ...`), and any `held by process item` lines from
  `wf next`.
- **Moved**: each step taken, its outcome, and the branch head it left.
- **Blocked on Jeff**: open blocking questions (options and recommendation),
  design reviews waiting (`*-design-review` steps, FLOW-19), open PRs waiting
  for his merge (phase 7), `escalate` steps, runaway budgets (FLOW-13),
  priority confirmation.
- **Design reviews skipped** this run (`design_review: skip`), each with the
  reason Design gave, so Jeff can call one back (FLOW-19).
- **Waiting on a host**: `host:<name>` steps and open native gaps, by
  machine (`hosts.yaml`), and any native run already in today's digest.
- **Spent**: steps this run against `run_budget.steps`, and each touched
  feature's spent against estimate, in steps and runs.
- **Notes**: FLOW-18 check result, whether Entire recorded the session.

Commit and push. End the session with the digest's text as the final message,
so the scheduled task's notification carries it.
