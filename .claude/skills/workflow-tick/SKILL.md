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

- Approve or enable auto-merge on any PR. Merge one only at `f7-merge` or
  the program sync, into a program trunk, with `--merge
  --match-head-commit`, after `wf check` passes (FLOW-08, LINT-03); never
  into a trunk, never a squash or rebase, never with `--admin`, and never
  dismiss a finding. An `f6-merged-<n>` whose work entry is on a program
  trunk is a loop merge too, under the same gate.
- Push `main` or a program trunk, force-push, rewrite pushed history, or
  delete a remote branch or a tag (FLOW-16, FLOW-17). The tags this skill
  pushes are the pre-releases `wf tag-next` names, `-dev.N` and `-rc.N`
  (FLOW-20). Opening a PR
  needs no confirmation (FLOW-08): `f6-open-pr` and a process item's
  `x2-implement` open theirs.
- Edit `docs/development/`, `.claude/`, or `tools/workflow/` anywhere but on
  a process item's branch after its design review (FLOW-14). The one
  exception: a rule change a feature's design review approved by name, made
  on that feature's branch in its approved `paths` (never `.claude/` or
  `tools/workflow/` outside `check/`, and never a loop rule or its
  enforcement, LOOP-09). Otherwise, on a feature branch, propose the change
  in the design for its review.
- Put secrets in prompts (FLOW-18).
- Change a trunk's or a program trunk's protection: on GitHub, its branch
  protection rule and the rulesets that apply to it (FLOW-08).
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
git fetch origin main workflow
test "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)"
(cd tools/workflow && go build -o ../../dist/wf ./cmd/wf)
git worktree add -B workflow .dev/workflow origin/workflow 2>/dev/null ||
  git -C .dev/workflow reset --hard origin/workflow
dist/wf validate -state .dev/workflow
```

The run takes the loop's tools from trunk (LOOP-09): if the checkout's HEAD
is not `origin/main`, stop before the build, push nothing, and report it as
the run's only output.

Then clone or fetch each repository an in-progress item names in `repos`,
other than this one, into `.dev/checkouts/<owner>/<name>` (ignored): `git
clone https://github.com/<owner>/<name>.git .dev/checkouts/<owner>/<name>`
the first time, `git -C .dev/checkouts/<owner>/<name> fetch --prune origin`
after. Pass `-checkouts .dev/checkouts` to every `wf` command below that takes
`-repo`. A repository that cannot be fetched (no access) stops that item's
steps: give it a `blocked` line naming the repository and go on with the
other items. Each repository's trunk is the one its `config.yaml`
`repositories` entry gives; `wf` reads it from there only (LOOP-01).

Keep any `limit:` lines `wf validate` prints for the digest (step 2.5
and step 4). The tick edits `.dev/workflow/areas.yaml` only while
`config.yaml` records a program trunk (FLOW-20), and then only for a change
that a program item's design, reviewed by Jeff or passed by the umbrella
test, names (a new repository's map, an area a move creates or empties, a
split, a mode), or that brings `reaches` in line with the dependency edges
(`tools/tier1/area-edges.sh | dist/wf areas -state .dev/workflow -check
-edges -`). Check the candidate map first with `dist/wf validate -state
.dev/workflow -areas <candidate> -repo <checkout>`, then
record each change, `dist/wf record -state .dev/workflow -worker $W -kind
area-map -ref <docs/design/<name>.md | area-edges> [-repository R] -note
"<the change and why>"`, and commit `areas.yaml` with
`program/records.yaml`: `wf commit` refuses a map change the push carries
without a new `area-map` record for it. With no program trunk configured,
the area map changes only on Jeff's word (FLOW-05).

`entire doctor --force` installs Entire's git hooks, which a fresh clone lacks;
`entire status` must name `buildwarden/buildwarden-sessions` as the
checkpoint remote. If `entire` is missing, carry on but make "sessions not recorded (FLOW-18)"
the digest's first line. Set `W=cloud/<UTC timestamp>` as this run's worker, at each invocation:
`wf claim` counts a run from the worker, on its first claim of each item.

Read `.dev/workflow/config.yaml`. Unattended and `priority_confirmed: false`:
write the digest (step 4) saying the loop waits on the human's confirmation of
the priority order, push it, and stop.

## 2. Take steps

While FLOW-20 stands (a `config.yaml` repositories entry has
`program_trunk`), a program item works against its repository's program
trunk: `dist/wf trunk -state .dev/workflow -feature F [-repository R]`
prints the branch an item forks from, merges, refreshes against and opens
its PR to. Before anything else in the run, each program item `wf status`
marks `first runaway: wf reestimate` gets `dist/wf reestimate -state
.dev/workflow -worker $W -feature F`, which raises its estimate to its spend
so far and records it (FLOW-13, FLOW-20); a second runaway, a fifth return
to Design, CI repair exhausted and an unresolved sync conflict are
escalations, `human` steps, named as escalations in the digest.

First, confirm merges, which spend no budget and need no claim: for each
pending `f6-merged-<n>` of a feature in Publish, each feature in phase 7
(`f7-merge`) and each process item in phase 3 (`x3-merge`), that the human
merges (`wf next` does not offer it to the cloud), run `dist/wf done
-state .dev/workflow -repo . -checkouts .dev/checkouts -worker $W -feature F
-step S`. It passes once the human has merged the pull request it confirms,
in each repository the item has one in, each against its own trunk
(FLOW-08); an `f6-merged-<n>` confirms only its work entry's, and the merge
step every one. It records each work entry's merge as its `merged`, and the
merge step moves the item to Close; otherwise leave it pending. `wf next`
never offers these steps (FLOW-05); this is where they complete, unless an
attended session already confirmed one (`workflow-human` item 5, FLOW-04):
only pending steps are confirmed, and `wf done` refuses a step already
done.

Then record each trunk (FLOW-08, FLOW-09): this repository's, and that of
each repository an in-progress item names in `repos`; and each program
trunk the same way, with `dist/wf main -state .dev/workflow [-repository
<owner>/<name>] -trunk program/componentization -sha <sha> -ci ...`, which
writes `program.yaml` beside `main.yaml` (the red-trunk hold is per trunk,
FLOW-10). For each, read the
check runs of its trunk's head (`gh api
repos/<owner>/<name>/commits/<sha>/check-runs`) and whether its ruleset
requires an up-to-date branch (`gh api
repos/<owner>/<name>/rules/branches/<trunk>`), record both, and pipe the
command its profile file's `areas.edges` names, run in its checkout, into the
area check. For this repository:

```bash
dist/wf main -state .dev/workflow -sha <sha> -ci green|red|pending \
  [-run <failing run's URL>] -up-to-date required|not-required
tools/tier1/area-edges.sh | dist/wf areas -state .dev/workflow -repo . -check -edges -
```

and for another, `dist/wf main -state .dev/workflow -repository <owner>/<name>
...` (its `repos/<owner>/<name>/main.yaml`) and `<its areas.edges> | dist/wf
areas -state .dev/workflow -repository <owner>/<name> -checkouts
.dev/checkouts -check -edges -`; a repository with no profile, or none naming
`areas.edges`, gets the check without `-edges`, and one with no map under
`repos/` none.

Keep the lines `wf areas -check` prints for the digest. On a red trunk
that `wf status` shows with no follow-up recorded, file the fix by the
`workflow-intake` skill's follow-up path (FLOW-09), in that repository, as
the follow-up of the item whose PR that merge was, and record it with
`dist/wf main -state .dev/workflow [-repository <owner>/<name>] -followup
<slug>`; file nothing when the merge was no item's, and list it under
**Blocked on Jeff**. Then `dist/wf stale -state .dev/workflow -repo .
-checkouts .dev/checkouts -worker $W`, which seeds a refresh step on each
item its own trunk has moved past, in any repository it has a branch in
(FLOW-10). Then the program sync (FLOW-20, Decision 14 of
`docs/design/program-north-star.md`): for each repository with a program
trunk, `dist/wf sync-program -state .dev/workflow -repo . -checkouts
.dev/checkouts -repository <owner>/<name>`; on `sync`, dispatch `wf-publish`
with step `sync`, the repository and the branch it names. The sync spends a
step of the budget and has no queue item; its merge, its conflict
resolution or its escalation is recorded with `wf record`.
Push what moved with `wf commit` (below): the `main.yaml` files and the
`queue/` files these wrote.

Every push of the workflow branch in this skill goes through `wf commit`,
never `git commit` and `git push`:

```bash
dist/wf commit -state .dev/workflow -worker $W -m "<message>" \
  [-claimed F/S] <paths changed>
```

It pushes only the paths given and the files it generates (FLOW-13). Exit 4:
the claim was asked again on the moved branch and refused (FLOW-04); go back
to 1 and ask `wf next` again. Exit 5: the change could not be replayed onto
the moved branch; `wf commit` names the files, and the change is re-applied
by hand on top of `origin/workflow` and committed again. Exit 5 with
`-claimed`: the push was rejected on every try and the claim was dropped; ask
`wf next` again, and never re-make the claim by hand. On
`main.yaml: kept the remote's; record main again`, run the `dist/wf main` and
`wf areas -check` lines above again for that repository, then `wf commit` its
`main.yaml`.

Then repeat while this run has taken fewer than `run_budget.steps` steps:

1. `dist/wf next -state .dev/workflow -worker $W` (attended: add
   `-feature <feature>`). `none` ends the loop; keep the `blocked` lines for
   the digest.
2. Claim it: `dist/wf claim -state .dev/workflow -worker $W -feature F -step S`,
   then
   `dist/wf commit ... -m "F: claim S" -claimed F/S queue/F.yaml`.
   Keep any line `wf claim` printed for a start past the waiting limit
   (FLOW-05) for the step's **Moved** line.
3. Check out the feature branch in the code checkout (`git switch <branch>`
   after a fetch; `f0-branch-setup` creates it), and in each clone of a
   repository the item's `work` names, where it exists. The trees must be
   clean. Close is the exception: it works on each trunk.
4. Dispatch with the Agent tool, `subagent_type` by step name:

   | Step | Agent |
   |---|---|
   | `f0-branch-setup`, `f6-merge-main`, `f6-consolidate`, `f6-open-pr` | `wf-publish` |
   | `f6-refresh`, `x2-refresh` (any `-r<n>`) | `wf-publish` |
   | `f7-merge`, `f6-merged-<n>` on a program trunk (`wf next` offers them only then) | `wf-publish` |
   | `f1-design`, a process item's cloud `x1-design` (FLOW-20) | `wf-design` |
   | `f2-umbrella`, `x2-process-test` | `wf-review`, in a fresh session that did not write the design |
   | `*-plan` | `wf-plan` |
   | `f3-implement`, `f3-implement-r<n>` | `wf-implement` |
   | `x2-implement` | `wf-implement`, then `wf-publish` to archive and open the PR |
   | `*-review` | `wf-review` |
   | `*-guide` | `wf-guide` |
   | `f8-close`, `x4-close` | none: do it inline, below |

   Close (FLOW-16) takes no PR and no human step, and is done in each
   repository the item has a branch in, in this checkout and each clone:
   confirm the branch head is in that repository's trunk, remove any worktree
   on the branch, `git branch -d` it (a squash-merged branch: `git branch -D`,
   only once `dist/wf check` has passed on it; never `-D` otherwise), clear
   the feature's validation leftovers outside the documented caches, then go
   to 5. If a remote branch is still there, release the step and add a
   `human` step `f8-escalate-branch` whose note gives the exact `git push
   origin --delete <branch>` command for each repository where it is (`git -C
   .dev/checkouts/<owner>/<name> ...` for another), and make `f8-close` need
   it.

   Steps may carry an `-r<n>` suffix after a return to an earlier phase. Give the agent
   the feature name, step id, the queue file path, the code checkout path, and
   the workflow checkout path. In Review, a red tier 1 goes to `wf-implement`
   for the fix and back to `wf-review`, inside the same step.
   At a loop merge (`f7-merge`, or an `f6-merged-<n>` on a program trunk)
   the agent runs `git fetch origin <program trunk> <branch>` in each
   repository it merged in, between `gh pr merge` and `dist/wf done`, so the
   merge proof reads the merged program trunk and not the ref from before
   the merge.
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
     `dist/wf done -state .dev/workflow -repo . -checkouts .dev/checkouts -worker $W -feature F -step S -run`.
     It runs the phase's done-when check (FLOW-03). If it fails, run
     `dist/wf release ... -why "<failed checks>"`; the next run retries.
   - `dist/wf validate -state .dev/workflow` must pass; fix what the agent
     wrote into the queue file if it does not. A `limit:` line that step 1
     already printed does not fail the step: it goes under **Blocked on
     Jeff**.
   - `dist/wf commit ... -m "F: <what happened>" <paths changed>` (FLOW-13:
     state after every step).
   - Each decision the step took in Jeff's place (a loop merge, a deferred
     or protected finding, an umbrella or process test, a security note
     read, a question settled as assumed, a departure from a decision of
     his on strategy, a return to Design past the second, a sync, a filed
     item, relation or area-map change, a tag): `dist/wf record -state
     .dev/workflow -worker $W -kind <kind> [-feature F] [-repository R]
     [-head <sha>] [-ref <ref>] -note "<what and why>"`, committed with
     `program/records.yaml` (FLOW-20).
   - After a loop merge on a program trunk: `dist/wf tag-next -state
     .dev/workflow -repo . -checkouts .dev/checkouts -repository R` names
     the `-dev.N` tags. When the merged item is the component's last phase
     in R (the last item of its phase row in `componentization.md` for that
     component, with no later program item for R filed), run it again with
     `-rc -feature F` too: it names the `-rc.N` once F's PR is in the
     program trunk's head, no other program item in R is open, the head's
     checks (its CI and conformance suites) passed, and every module R
     consumes is pinned at the latest pre-release in its clone; a pin that
     is behind is moved by a reconciliation item first (LOOP-07). For each
     tag either prints, `git tag <tag> origin/<program trunk>` and `git
     push origin <tag>` in that repository, and `wf record -kind tag`. It
     prints `none` with the reason otherwise. Never move or delete a tag.
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
  step, its outcome, and the branch head it left in each repository (the
  repository named when it is not this one, LOOP-06); for an
  `f2-plan` step, also the plan's task count, so Jeff can call a review back
  (FLOW-19). A step whose claim started a feature past the waiting limit
  carries the line `wf claim` printed (FLOW-05). A step that changed a specification adds a **Specification
  changes** line under its own, naming each one apart from the code (LOOP-02).
  A review step's line gives tier 1's result and the tier-2 findings counted
  by severity, with the report's path (LINT-05). Each lead this run set or
  cleared (`wf new -lead`, `wf lead`) is a line of its own, from the item's
  history line (FLOW-04).
- **Held**: one line, the items `QUEUE.md` lists under **Held** (FLOW-20);
  a held item is never listed under **Blocked on Jeff**.
- **Decided for you**: the run's records (`dist/wf record -state
  .dev/workflow -list -since <run start>`, which lists a departure from his
  decisions, a return past the second and a re-estimate first), so Jeff
  can call any back (FLOW-20).
- **Blocked on Jeff**: each step named with its kind, setup, design or
  privileged, or as an escalation (FLOW-20): open blocking questions
  (options and recommendation),
  design reviews waiting (`*-design-review` steps, FLOW-19), each open PR
  to a trunk, as ready to merge or as waiting for a refresh (FLOW-08,
  FLOW-10); a PR the loop merges into a program trunk is not listed, but one
  held at `f7-merge` on a security note or a finding only Jeff may clear
  is,
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
  reason Design gave, so Jeff can call one back (FLOW-19); and each
  umbrella test and process test passed this run, with its line.
- **Waiting on a host**: `host:<name>` steps and open native gaps, by
  machine (`hosts.yaml`), each led item with its lead (`wf status`'s
  `lead:` line; `wf next` reports a queued one as waiting on its lead's
  step), and any native run already in today's digest.
- **Spent**: steps this run against `run_budget.steps`, and each open item
  the run touched with its steps and runs spent against estimate, nothing
  flagged as over budget. An item this run closed (`f8-close`, `x4-close`)
  is in **Moved**, not here (FLOW-13).
- **Notes**: FLOW-18 check result, whether Entire recorded the session.

`dist/wf commit ... -m "digest: run $W" <the run's file>`, which also commits
the day's page. End the session with the run's section as the final message,
so the scheduled task's notification carries it.
