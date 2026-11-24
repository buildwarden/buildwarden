---
name: workflow-native
description: A native-validation session on a native machine (macbook, windows-desktop). Drains that machine's host steps (macos-vz, macos-qemu, macos-finch, windows-hyperv, qemu-any, container-any, ...) and native-gap backlog in one batch, testing main plus every waiting branch, and takes the steps of items its host names lead. Run as "/workflow-native <host> [<machine>]" attended, or as "/workflow-native unattended <machine>" by the scheduled native session.
---

# Native-validation session

The `host:<name>` worker of FLOW-04, doing tier-2 validation that CI cannot
(FLOW-09, TEST-02, TEST-03), and the steps of an item its host name **leads**:
Branch setup through Guide of a feature whose `lead` is that host (FLOW-04,
FLOW-09). The procedure is here; the rules are there (FLOW-14). Which host
names a machine serves is `hosts.yaml` on the workflow branch; a name may be
listed on several machines, and a step of it goes to whichever takes it first.

Arguments:

- `<host> [<machine>]`: attended, one host name; ask which host this is if it
  is missing. The machine is the one `hosts.yaml` lists for the host when only
  one does, and otherwise the second argument; with several machines and none
  named, ask which machine this is.
- `unattended <machine>`: started by `tools/workflow/native/native-tick.sh`
  after `wf gate` found work for the machine. Every host the machine serves in
  `hosts.yaml`; nobody is watching. When the prompt names **stale hosts** (a
  cached asset such as the Hyper-V relay boot disk failed to refresh), take no
  step or backlog entry for those hosts this run, work the machine's other
  hosts as usual, and say in the digest which hosts were skipped and why. When
  the prompt says **sessions not recorded** (FLOW-18), take no led step this
  run, validate as usual, and put that line first in the digest file. When it
  names **leftovers** (a `.scratch/` worktree or branch the wrapper's cleanup
  kept, FLOW-16), list them under **Notes** in the digest. When it says
  **Unfinished** (an earlier run ended unfinished, and the wrapper saved its
  workflow changes under `.scratch/leftover/<run>/`: `uncommitted.patch`,
  `untracked.tar` and `commits/`), read them, record the evidence they hold
  on their steps and backlog entries as `dist/wf done`, `dist/wf release` or
  a backlog note, and name the run under **Notes**; the wrapper released
  that run's leases, and removes the directory once this run ends finished.
  With nothing else to do, end the run without claiming anything.

## Never (unattended)

The `workflow-tick` skill's Never list applies, without its loop merges: no
merge of any PR, into a trunk or a program trunk, no approval or
auto-merge (`settings.native.json` denies `gh pr merge`); no push to `main`
or a program trunk, no tag, force-push or remote branch delete; no edits to
`docs/development/`, `.claude/` or `tools/workflow/`, except a rule change the
feature's design review approved, in its `paths` (FLOW-14), and never a loop
rule or its enforcement (LOOP-09); no secrets in prompts; no change to a
trunk's or a program trunk's protection (on GitHub, its branch protection
rule and the rulesets that apply to it, FLOW-08); no urgent mark on an item
(FLOW-05).

Also: never take `human` or `cloud` steps, never ask a question and wait (record
it in the queue and the digest instead), and never change anything on the
machine outside the checkout, the documented caches, and the VMs, images and
containers this session creates. And:

- never end the run while a command this session started in the
  background is still running: a command that fits the Bash tool's
  timeout runs in the foreground; a longer one is waited for, in bounded
  waits, until it exits, or stopped and its case recorded as not run
  (release the step, or keep the backlog entry open, with the reason),
  before the digest is written. The wrapper counts a run whose digest it
  cannot find, or that left a background process, as unfinished;

- no step of an item with an open blocking question or a spent runaway
  budget, which `wf next` now enforces for every worker but a human (FLOW-07,
  FLOW-13);
- no push but by `dist/wf commit` (the workflow branch) and
  `tools/workflow/native/native-push.sh <name>` (a feature branch, FLOW-17),
  or `native-push.sh <name> <owner>/<repo>` for a branch in a second
  repository's clone;
- no `git -C ... commit -m`: a commit message goes in `.scratch/<name>.msg`
  and is committed with `git -C .scratch/<name> commit -F ../<name>.msg`,
  never `-m`, and never `git -C ... log --grep push`, because the deny
  `Bash(git -C * push*)` refuses any `git -C` command whose text contains
  ` push`.

## Git on the workflow checkout (unattended)

Nobody is there to approve a prompt, and `settings.native.json` allows git on
the workflow checkout only in this form, run from the checkout root:
`git -C .dev/workflow <add|commit|...>`. The workflow branch is pushed only
by `dist/wf commit` (**Record**), which runs git itself, and a feature branch
only by `tools/workflow/native/native-push.sh <name>`, since the denies
`Bash(git push*)` and `Bash(git -C * push*)` refuse every other push. Never an
absolute path, and never `cd .dev/workflow && git ...`. A branch under test
goes in a worktree under `.scratch/` (`git worktree add .scratch/<name> <ref>`),
driven with `git -C .scratch/<name> ...`, with its binaries run as
`.scratch/<name>/dist/warden ...` and its targets with
`make -C .scratch/<name> <target>`, all from the checkout root. Its scripts
run as
`tools/workflow/native/native-run.sh -- .scratch/<name>/tools/<path> ...`
(never its `tools/workflow/`), and a command that needs a `WARDEN_*`
variable as `tools/workflow/native/native-run.sh WARDEN_X=value -- <command>`,
never as `WARDEN_X=value <command>`. A command the procedure needs that is
outside those forms and still refused goes in a scratch Makefile under
`.scratch/<name>/`, run with `make -C .scratch/<name> <target>`; the digest
names the target and the exact commands its recipe runs, and lists it under
**Blocked on Jeff** as an allowlist gap. A case neither form can run is
handled as **Record** says. A session that still cannot commit or push
leaves its changes for the wrapper, which saves them and reports the run as
failed.

## Set up

On the machine itself. The FLOW-18 check, `dist/wf` and the `.dev/workflow`
checkout as in `workflow-tick` step 1; the scheduled wrapper has already done
this. Workers: one per host. Unattended, `W=host:<name>/<machine>.<run>`, the
session id in the prompt being `<machine>.<run>`; the wrapper releases every
lease it still holds when it ends. Attended,
`W=host:<name>/<machine>.attended-<UTC timestamp>`, with the machine from
**Arguments**; its **Close** releases its leases. A claim holds a lease of
`host_lease_minutes` (20 hours when unset, FLOW-04).

The machine's clones of the other repositories `hosts.yaml` lists for it
are the owner's, at `.dev/checkouts/<owner>/<repo>` (LOOP-05): the wrapper
has reset each to its trunk; attended, check that each is there, clean and
on its trunk (`dist/wf native -state .dev/workflow -repo . -machine <machine>
-check`), and never clone one. Pass `-checkouts .dev/checkouts` to every `wf`
command that takes `-repo`. `wf next` offers this machine only the steps
of repositories it lists (FLOW-04).

## Collect the batch

For each host this session serves:

- Queue steps, validation and led steps alike: repeat
  `dist/wf next -state .dev/workflow -worker $W` and claim each
  (`dist/wf claim`), pushing each claim with
  `dist/wf commit -state .dev/workflow -worker $W -m "F: claim S" -claimed F/S`.
  Exit 4: the claim was asked again on the moved branch and refused
  (FLOW-04); ask `wf next` again. Exit 5: as `workflow-tick` step 2 says.
  A led step is one of an item whose `lead` is this host (`wf status` prints
  a `lead:` line); none is claimed in a run whose prompt says sessions are
  not recorded.
- Backlog: every `open` entry for this host in `backlog/native-gaps.yaml`.
  A `deferred` entry is not retried: its note says what it waits on.

Unattended, the batch is bounded by the native budget,
`dist/wf budget -state .dev/workflow -native` (`native_budget` in
`config.yaml`, falling back to `run_budget`; FLOW-13), counting claimed steps
and backlog entries together, taken in FLOW-05 order
for steps and file order for the backlog. Queue steps come before the backlog.

## Work

Validation steps and backlog entries: test in one sitting, `main` first,
then each program trunk (`program/componentization`, while FLOW-20 stands),
then each waiting branch (FLOW-09), each against its own trunk (`dist/wf
trunk -state .dev/workflow -feature F`), using the commands each design's Validation
plan names and `make deadcode` where the backlog asks for it.

Led steps (Branch setup through Guide of an item this host leads) and any
other design or implementation step with host affinity: check out the
feature branch in a worktree `.scratch/<item>`, never in the wrapper's
checkout, which stays on `main`:

- Branch setup: run `dist/wf trunk -state .dev/workflow -feature <item>`
  first and use its first field, `<trunk>`, literally (no command
  substitution, which the allowlist matcher cannot see through): `git
  worktree add .scratch/<item> -b feat/<item> origin/<trunk>`. A program
  item's trunk is its program trunk (FLOW-20).
- A later step, no worktree yet: `git worktree add .scratch/<item>
  feat/<item>`.
- A worktree kept from an earlier night (the wrapper keeps it while the
  item is open): no `worktree add`; `git -C .scratch/<item> merge
  --ff-only origin/feat/<item>`. A refused fast-forward (local commits not
  pushed) is pushed first with `native-push.sh`, or the step released with
  the reason; never a reset or a force-push (FLOW-17).

Dispatch to
the phase agent as in `workflow-tick` step 2.4, telling it the worktree, and
finish as in its step 2.5. On this machine every commit of the branch is
`git -C .scratch/<item> commit -F ../<item>.msg`, the message written to
`.scratch/<item>.msg` first, and every push, at each green checkpoint
(FLOW-17), is `tools/workflow/native/native-push.sh <item>`, which pushes
only the checked-out `feat/<item>` of an item in progress, fast-forward only.
A branch of the item in a second repository it works in is worked in that
repository's clone, after `git -C .dev/checkouts/<owner>/<repo> fetch
origin`:

- Branch setup: `dist/wf trunk -state .dev/workflow -feature <item>
  -repository <owner>/<repo>`, its first field used literally as above, then
  `git -C .dev/checkouts/<owner>/<repo> worktree add -b feat/<item>
  ../../../../.scratch/<owner>/<repo>/<item> origin/<trunk>`.
- A later step, no worktree yet: `git -C .dev/checkouts/<owner>/<repo>
  worktree add ../../../../.scratch/<owner>/<repo>/<item> feat/<item>`.
- A worktree kept from an earlier night: no `worktree add`; `git -C
  .scratch/<owner>/<repo>/<item> merge --ff-only origin/feat/<item>`, a
  refused fast-forward handled as above.

It is committed with `git -C .scratch/<owner>/<repo>/<item> commit -F
../<item>.msg` (the message at `.scratch/<owner>/<repo>/<item>.msg`) and
pushed with `tools/workflow/native/native-push.sh <item> <owner>/<repo>`;
its scripts run as `tools/workflow/native/native-run.sh --
.scratch/<owner>/<repo>/<item>/tools/<path> ...`, as for this repository's.
No other git command runs in the clone, which the wrapper resets to its
trunk at its next run.

Keep the lease while working: before each phase-agent dispatch, and at each
checkpoint push, when less than half the lease is left, renew it with
`dist/wf renew -state .dev/workflow -worker $W -feature F -step S` and push
the renewal with
`dist/wf commit -state .dev/workflow -worker $W -m "F: renew S" -renewed F/S`.
A renewal counts nothing against the item's budget. Exit 4: the step was
released or taken on the moved branch; stop working on it and ask `wf next`
again. A step this session stops working on without finishing it is
released with `dist/wf release` and the reason, and pushed with
`dist/wf commit`.

A lead may be set or changed by any worker with
`dist/wf lead -state .dev/workflow -worker $W -feature F -host <name>` (or
`-clear`) when a design or a finding shows the item needs another host; the
command writes the history line, and the digest lists it under **Moved**.

## Record

- A queue step that passes: `dist/wf done ... -checkouts .dev/checkouts
  -step S` with the evidence (commands, versions, short log excerpt) in the
  queue history.
- A failure: attach the logs under `native/<host>/<date>/` on the workflow
  branch. If the feature's branch causes it, release the step with the
  failure as the reason and return the feature with `dist/wf back`, to
  Design for a design gap or Implement for a bug (FLOW-06, FLOW-09).
- A regression `main` already has (it fails on `main` too, side findings
  included): file it at once as the feature's follow-up by the
  `workflow-intake` skill's follow-up path, attended or not, and name it in
  the step's evidence as "<slug> (follows <feature>)" (FLOW-09). The step
  itself passes or fails on the feature's own cases.
- A step `wf done` refuses as a later-phase step (the feature was returned
  while it ran, FLOW-04, FLOW-06): release it with the evidence as the
  reason and leave it pending; it is offered again once the feature is back
  in its phase.
- A case the allowlist refused and no scratch Makefile could run: release
  the step with the refused command as the reason and leave it pending
  (never `wf done` with the case skipped); a backlog entry stays open with
  the same note.
- Backlog entries: set `status: done` with the date and evidence, or keep them
  open with what was found.
- `dist/wf validate -state .dev/workflow` must pass before each push.
- Every push of the workflow branch goes through
  `dist/wf commit -state .dev/workflow -worker $W -m "<message>" <paths>`,
  as in `workflow-tick` step 2, never `git commit` and `git push`.

## Close

Clean up test images, VMs, containers and scratch output outside the documented
caches (FLOW-16); a led item's `.scratch/<item>` worktree stays while the item
is open, and the wrapper removes it once the item is done or its branch is
gone (FLOW-16). Switch the checkout back to `main`. Push the workflow
branch with `dist/wf commit`; never push `main` or a program trunk, or merge
anything (FLOW-08, FLOW-17).

Attended: release every lease the session still holds with
`dist/wf release -state .dev/workflow -session <machine>.attended-<UTC> -why
"attended session closed"` and push it with `dist/wf commit`; the wrapper's
release of stale leases never frees an attended session's, so a lease left
here runs to its expiry. Finish with a summary per branch.

Unattended: write the run's own file,
`.dev/workflow/digest/<YYYY-MM-DD>/<run>-host-<machine>.md`
(America/Los_Angeles date; `<run>` is the part of the session id after
`<machine>.`, e.g. `20261122T124005Z-host-macbook.md`, which is how the
wrapper finds it), starting with a
`## Run host:<machine>/<timestamp>` heading (after the "sessions not recorded
(FLOW-18)" line when the prompt has it), and run
`dist/wf digest -state .dev/workflow`, which builds the day's page; checked
against LOOP-06's list before the commit, with
**Moved** (per host and branch: what ran, pass or fail, evidence; for each
queue step, the item with its kind and FLOW-05 class, and the branch head it
left; a **Specification changes** line under a step that
changed a specification, LOOP-02; on a review step's line, tier 1's result and
the tier-2 findings counted by severity, with the report's path, LINT-05;
each lead this run set or cleared, with `wf new -lead` or `wf lead`, from its
history line),
**Blocked on Jeff** (anything this machine cannot do: a missing tool, a
failure that needs a decision), **Follow-ups filed** (each as
"<slug> (follows <item>)", so Jeff can say otherwise; "none" when none),
**Spent** (steps and
backlog entries against the native budget, a led step counting one like a
validation step, FLOW-13), and **Notes** (FLOW-18 check, whether Entire
recorded the session, free disk at the end, the wrapper's leftovers). Push it with
`dist/wf commit`, which also commits the day's page.
If a feature was unblocked or returned to Design, or a backlog entry failed,
notify Jeff (the PushNotification tool when available) naming the feature and
outcome. End with the digest section as the final message.
