---
name: workflow-tick
description: One run of BuildWarden's unattended feature loop (FLOW-13). Takes cloud steps from the workflow-branch queue in FLOW-05 order, dispatches each to its phase agent, writes state after every step, and writes the run's digest. Run by the daily scheduled task, or by hand as "/workflow-tick attended <feature>".
---

# Workflow tick

One run of the loop in FLOW-13. The rules are in `docs/development/process.md`
(FLOW-03..FLOW-18), `docs/development/behavioral-linter.md` and
`docs/development/loop-contract.md` (LOOP-02, LOOP-06, LOOP-09); this skill is
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
  a process item's branch after its design review (FLOW-14). The one
  exception: a rule change a feature's design review approved by name, made
  on that feature's branch in its approved `paths` (never `.claude/` or
  `tools/workflow/` outside `check/`, and never a loop rule or its
  enforcement, LOOP-09). Otherwise, on a feature branch, propose the change
  in the design for its review.
- Put secrets in prompts (FLOW-18).
- Change a trunk's protection: on GitHub, its branch protection rule and
  the rulesets that apply to it (FLOW-08).
- Mark an item urgent (FLOW-05).

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
dist/wf validate -state .dev/workflow
```

Keep any `limit:` lines `wf validate` prints for the digest (step 2.5
and step 4). The tick never edits `.dev/workflow/areas.yaml`: the
area map changes only on Jeff's word (FLOW-05).

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
Close; otherwise leave it pending. `wf next` never offers these steps
(FLOW-05); this is where they complete.

Then record `main` (FLOW-08, FLOW-09). Read the check runs of `origin/main`'s
head (`gh api repos/buildwarden/buildwarden/commits/<sha>/check-runs`) and
whether its ruleset requires an up-to-date branch
(`gh api repos/buildwarden/buildwarden/rules/branches/main`), and record
both:

```bash
dist/wf main -state .dev/workflow -sha <sha> -ci green|red|pending \
  [-run <failing run's URL>] -up-to-date required|not-required
tools/tier1/area-edges.sh | dist/wf areas -state .dev/workflow -repo . -check -edges -
```

Keep the lines `wf areas -check` prints for the digest. On a red `main`
that `wf status` shows with no follow-up recorded, file the fix by the
`workflow-intake` skill's follow-up path (FLOW-09), as the follow-up of the
item whose PR that merge was, and record it with
`dist/wf main -state .dev/workflow -followup <slug>`; file nothing when the
merge was no item's, and list it under **Blocked on Jeff**. Then
`dist/wf stale -state .dev/workflow -repo . -worker $W`, which seeds a
refresh step on each item `main` has moved past (FLOW-10). Push what moved
with `wf commit` (below): `main.yaml` and the `queue/` files these wrote.

Every push of the workflow branch in this skill goes through `wf commit`,
never `git commit` and `git push`:

```bash
dist/wf commit -state .dev/workflow -worker $W -m "<message>" \
  [-claimed F/S [-new-run]] <paths changed>
```

It pushes only the paths given and the files it generates (FLOW-13). Exit 4:
the claim was asked again on the moved branch and refused (FLOW-04); go back
to 1 and ask `wf next` again. Exit 5: the change could not be replayed onto
the moved branch; `wf commit` names the files, and the change is re-applied
by hand on top of `origin/workflow` and committed again. Exit 5 with
`-claimed`: the push was rejected on every try and the claim was dropped; ask
`wf next` again, and never re-make the claim by hand. On
`main.yaml: kept the remote's; record main again`, run the `dist/wf main` and
`wf areas -check` lines above again, then `wf commit` `main.yaml`.

Then repeat while this run has taken fewer than `run_budget.steps` steps:

1. `dist/wf next -state .dev/workflow -worker $W` (attended: add
   `-feature <feature>`). `none` ends the loop; keep the `blocked` lines for
   the digest.
2. Claim it: `dist/wf claim -state .dev/workflow -worker $W -feature F -step S`,
   adding `-new-run` on this run's first claim of F, then
   `dist/wf commit ... -m "F: claim S" -claimed F/S [-new-run] queue/F.yaml`.
   Keep any line `wf claim` printed for a start past the waiting limit
   (FLOW-05) for the step's **Moved** line.
3. Check out the feature branch in the code checkout (`git switch <branch>`
   after a fetch; `f0-branch-setup` creates it). The tree must be clean.
   Close is the exception: it works on `main`.
4. Dispatch with the Agent tool, `subagent_type` by step name:

   | Step | Agent |
   |---|---|
   | `f0-branch-setup`, `f6-merge-main`, `f6-consolidate`, `f6-open-pr` | `wf-publish` |
   | `f6-refresh`, `x2-refresh` (any `-r<n>`) | `wf-publish` |
   | `f1-design` | `wf-design` |
   | `*-plan` | `wf-plan` |
   | `f3-implement`, `f3-implement-r<n>` | `wf-implement` |
   | `x2-implement` | `wf-implement`, then `wf-publish` to archive and open the PR |
   | `*-review` | `wf-review` |
   | `*-guide` | `wf-guide` |
   | `f8-close`, `x4-close` | none: do it inline, below |

   Close (FLOW-16) takes no PR and no human step: confirm the branch head is
   in `origin/main`, remove any worktree on the branch, `git branch -d` it
   (a squash-merged branch: `git branch -D`, only once `dist/wf check` has
   passed on it; never `-D` otherwise), clear the feature's
   validation leftovers outside the documented caches, then go to 5. If the remote branch is still there,
   release the step and add a `human` step `f8-escalate-branch` whose note
   gives the exact `git push origin --delete <branch>` command, and make
   `f8-close` need it.

   Steps may carry an `-r<n>` suffix after a return to an earlier phase. Give the agent
   the feature name, step id, the queue file path, the code checkout path, and
   the workflow checkout path. In Review, a red tier 1 goes to `wf-implement`
   for the fix and back to `wf-review`, inside the same step.
5. Finish the step on the workflow checkout:
   - The agent returned `merged meanwhile` (Jeff merged the PR while a
     refresh ran, FLOW-10): end the step with the merge confirmation above
     (`dist/wf done` on the item's merge step), not with `wf done` on the
     refresh, which the confirmation drops.
   - The agent returned `back-to-design`:
     `dist/wf back -state .dev/workflow -worker $W -feature F -why "<finding>"`.
     Consolidation returning work the plan already covers adds `-to 3`
     (back to Implement, FLOW-10).
   - Otherwise:
     `dist/wf done -state .dev/workflow -repo . -worker $W -feature F -step S -run`.
     It runs the phase's done-when check (FLOW-03). If it fails, run
     `dist/wf release ... -why "<failed checks>"`; the next run retries.
   - `dist/wf validate -state .dev/workflow` must pass; fix what the agent
     wrote into the queue file if it does not. A `limit:` line that step 1
     already printed does not fail the step: it goes under **Blocked on
     Jeff**.
   - `dist/wf commit ... -m "F: <what happened>" <paths changed>` (FLOW-13:
     state after every step).
6. If the step raised a new blocking question, notify the human now (the
   PushNotification tool when available), naming the feature and question.

## 3. Close

`tools/workflow/flow18-check.sh` again. Switch the code checkout back to
`main`. Leave no uncommitted work on any branch.

## 4. Digest

Write the run's own file, `.dev/workflow/digest/<YYYY-MM-DD>/<UTC start>-<W>.md`
(the date in America/Los_Angeles, `/` in `W` written as `-`), starting with a
`## Run <W>` heading, then run `dist/wf digest -state .dev/workflow`, which
builds the day's page from the day's run files. The file is the run's report
(LOOP-06): check it against that rule's list before the commit. Sections, in
order:

- **Process items** first, when any is queued or in progress (FLOW-03,
  FLOW-05): each one's phase and what it holds (`dist/wf status` prints
  `process item, holds: ...`), and any `held by process item` lines from
  `wf next`. Only queued and in-progress ones; one this run closed goes in
  **Moved**.
- **Moved**: each step taken: the item, its kind and FLOW-05 class, the
  step, its outcome, and the branch head it left; for an
  `f2-plan` step, also the plan's task count, so Jeff can call a review back
  (FLOW-19). A step whose claim started a feature past the waiting limit
  carries the line `wf claim` printed (FLOW-05). A step that changed a specification adds a **Specification
  changes** line under its own, naming each one apart from the code (LOOP-02).
  A review step's line gives tier 1's result and the tier-2 findings counted
  by severity, with the report's path (LINT-05).
- **Blocked on Jeff**: open blocking questions (options and recommendation),
  design reviews waiting (`*-design-review` steps, FLOW-19), each open PR,
  as ready to merge or as waiting for a refresh (FLOW-08, FLOW-10),
  `escalate` steps, an item sent back with its PR open, a red `main`
  (FLOW-09), the `limit:` lines and holders over a limit (FLOW-05), the
  lines of `wf areas -check`, runaway budgets, priority confirmation. List a runaway budget only as `QUEUE.md`'s **Needs Jeff**
  does, one line per open item it names (FLOW-13); do not judge budgets
  yourself, and never call a run overrun a runaway.
- **Follow-ups filed**: each follow-up a step filed this run (FLOW-09), as
  "<slug> (follows <item>)", so Jeff can say otherwise; "none" when none. A
  follow-up does not wait on him, so it is not listed under **Blocked on
  Jeff**.
- **Design reviews skipped** this run (`design_review: skip`), each with the
  reason Design gave, so Jeff can call one back (FLOW-19).
- **Waiting on a host**: `host:<name>` steps and open native gaps, by
  machine (`hosts.yaml`), and any native run already in today's digest.
- **Spent**: steps this run against `run_budget.steps`, and each open item
  the run touched with its steps and runs spent against estimate, nothing
  flagged as over budget. An item this run closed (`f8-close`, `x4-close`)
  is in **Moved**, not here (FLOW-13).
- **Notes**: FLOW-18 check result, whether Entire recorded the session.

`dist/wf commit ... -m "digest: run $W" <the run's file>`, which also commits
the day's page. End the session with the run's section as the final message,
so the scheduled task's notification carries it.
