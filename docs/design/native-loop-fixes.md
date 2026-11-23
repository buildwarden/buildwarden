---
feature: native-loop-fixes
status: draft
drivers: []
seams: []
surfaces: []
repos: []
merge_first: []
areas: [loop, rules]
security_boundary: none
alters_relationships: false
design_review: required
native_validation:
  - {host: macos-any, blocking: false, reason: "The wrapper, the install check and the launcher run from main only (the dedicated checkout resets to origin/main), so the macbook's first scheduled runs after the merge exercise them; tier 0 covers the Go and the launcher's cases."}
  - {host: windows-any, blocking: false, reason: "The same wrapper under Git Bash: the stamp, digest check, fingerprint and cleanup changes reach it; the PATH and pmset fixes are Darwin-only."}
tier2_gaps:
  - "macos-any: an unattended run that ends with background work, and one with a program-trunk-only move, on the macbook after the merge (backlog native-loop-fixes-macbook)"
  - "windows-any: the stamp, digest and cleanup changes under Git Bash after the merge (backlog native-loop-fixes-windows)"
rules: [FLOW-04, FLOW-09, FLOW-13, FLOW-14, FLOW-16, FLOW-17, FLOW-18, FLOW-19, FLOW-20, LOOP-05, LOOP-09, PROFILE-01, GLOBAL-09, GLOBAL-10, GLOBAL-11]
rule_changes:
  - {rule: FLOW-09, change: amend, paths: [docs/development/process.md]}
  - {rule: PROFILE-01, change: amend, paths: [docs/development/loop-profile.md, docs/development/loop-profile.yaml]}
concepts: [workflow, native-validation, scheduled-sessions, gate, program-trunk, scratch-worktree, second-repository, machine-cleanup, session-recording, native-session]
related: [native-host-loop.md, native-loop-followups.md, loop-native-lead.md, loop-repository.md, native-allowlist-scratch-tools.md, native-refresh-scoped.md]
---

# Native loop fixes: an unattended session finishes its background work, the gate and cleanup follow the program trunks, and a second repository's steps run as the skill writes them

## Problem

Jeff's attended macbook session of 2026-11-23, a readiness survey of the
Mac native loop for the componentization program, found eleven defects.
He asked for one consolidated bugfix: one design, one PR, no
case-by-case rounds. Each is confirmed below against `main` at 168cf95.

| # | Defect | Confirmed at |
|---|---|---|
| 1 | Unattended `-p` sessions end their turn while `run_in_background` builds still run, and exit with them running (runs 20261120T125213Z, 600 s background-wait ceiling, and 20261123T124004Z). The skill forbids only "ask a question and wait". | `.claude/skills/workflow-native/SKILL.md`, **Never (unattended)**: "never ask a question and wait"; nothing on background work. Status history: 2 of the 7 macbook runs 2026-11-17..23 failed this way. |
| 2 | The wrapper moves `gate-macbook.json` past such a run and hands its leftover to no later session. | `native-tick.sh:412-414` moves the stamp on `rc 0` alone; the leftover check runs later, in `on_exit` → `record_status` → `reset_workflow` (`:127`), and saves to `$state/leftover/<run>/`, outside the checkout, which no prompt names and the session cannot read (backlog notes: reads outside the checkout are blocked). |
| 3 | Branch setup forks from `origin/main`; a program item's branch forks from `wf trunk`. | SKILL.md **Work**: "`-b feat/<item> origin/main` at Branch setup". `wf trunk` (`cmd/wf/main.go:641`) gives `program/componentization` for a program item (FLOW-20). |
| 4 | The second-repository procedure has no Branch setup form and no form for a worktree kept from an earlier night. | SKILL.md **Work** gives only `worktree add ../../../../.scratch/<owner>/<repo>/<item> feat/<item>`: no `-b`, no base; on a kept worktree `worktree add` fails ("already exists"), and no reuse form is given. The same holds for this repository's `.scratch/<item>`. |
| 5 | `native-run.sh` refuses `.scratch/<owner>/<repo>/<item>/{tools,dist}` paths and `go -C` into them. | `native-run.sh:96-116`: `name=${rest%%/*}` takes `<owner>` as the worktree, then `.scratch/<owner>` is not a worktree, and the common-dir check (`:116`) requires this repository's; `:126-130` refuses `dist/` under three components; `:142` `valid_name` refuses a `/` in `go -C`'s directory. |
| 6 | The gate fingerprint hashes each repository's trunk, never its program trunk, so a program-trunk move wakes no native machine. | `queue/hosts.go:430`: `refs := []string{"refs/remotes/origin/" + trunk}` plus in-progress branches; `ProgramTrunkOf` is never read there. |
| 7 | Cleanup's `branch -d` tests a program item's branch against `main` once its remote branch is gone, leaving a FLOW-16 leftover note every night; detached validation worktrees and non-worktree `.scratch` directories are neither removed nor named. | `native-tick.sh:224`: `git -C "$clone" branch -d "$branch"` with no upstream left tests against the clone's `HEAD`, its trunk. `:205` `[ -f "$d.git" ] \|\| continue` and `:206` `symbolic-ref ... \|\| continue` skip a non-worktree directory and a detached worktree silently. |
| 8 | The wrapper's PATH lacks the rustup proxies the relay needs; linked into `~/.local/bin` on the macbook as a stopgap. | `native-tick.sh:55`: `~/.local/bin`, mise shims, `/opt/homebrew/bin`, `/usr/local/bin` only; Homebrew's `rustup` keeps its proxies in `/opt/homebrew/opt/rustup/bin` (keg-only), `rustup-init` in `~/.cargo/bin`. |
| 9 | `entire status` reports the Claude Code hooks in `.claude/settings.json` out of date. | Reproduced here with Entire CLI 0.12.2: `! Claude Code hooks out of date · run 'entire doctor'`. In a throwaway copy, `entire agent add claude-code` changes one line: the SessionStart command drops its `entire --version` probe. Nothing else in the file changes. |
| 10 | `pmset -g ... \| grep -q` under `pipefail` exits 141: `install-macos.sh:135` falsely warns of no scheduled wake; `native-tick.sh:286`'s AC-power check can falsely skip a run. | `install-macos.sh:13` `set -uo pipefail`, `:135` `pmset -g sched \| grep -qi wake`; `native-tick.sh:39` `set -euo pipefail`, `:286` `! pmset -g batt \| grep -q "AC Power"`. `grep -q` exits on the first match, `pmset` takes SIGPIPE on its next write, and `pipefail` makes the pipeline 141. `install-macos.sh:100` (`printf \| grep -Eq`) has the same shape. |
| 11 | `loop-profile.yaml` has no `native.check` for the macbook's hosts (finch VM running, qemu present, warden signed with the virtualization entitlement), and `wf native -check` does not check the clones' Entire git hooks. | `docs/development/loop-profile.yaml`: `native.check` has `windows-hyperv` only. `cmd/wf/native.go:186-232`: origin, cleanliness, `toolchain_check`, `native.check`; no hook check. |

Out of scope, at Jeff's word: an allow rule for `cargo`. A scratch
Makefile covers the relay's steps; a new permission is his under FLOW-20.

## Prior art

- [`native-host-loop`](native-host-loop.md): the wrapper, the gate and its
  stamp. This design keeps the stamp's meaning ("what the last full
  session saw") and makes "full" checkable.
- [`native-loop-followups`](native-loop-followups.md): the status file and
  the leftover save. Fix 2 hands that save to the next session, where
  today it only reaches a human.
- [`loop-native-lead`](loop-native-lead.md): the machine cleanup (its
  `leftover` notes, `git worktree remove` and `git branch -d` only, a
  squash-merged branch kept). Kept, with the program trunk as the
  upstream `branch -d` tests against (fix 7).
- [`loop-repository`](loop-repository.md) a14: the clones at
  `.dev/checkouts/<owner>/<repo>`, worktrees at
  `.scratch/<owner>/<repo>/<name>`, and a fingerprint of "each
  repository's trunk and in-progress branches". Fixes 4, 5 and 6 complete
  it; the program trunks came later (FLOW-20) and a14 predates them.
- [`native-allowlist-scratch-tools`](native-allowlist-scratch-tools.md):
  why `native-run.sh` exists, and that its checks are shape checks, not
  a sandbox ("make, go and powershell already run anything"). Fix 5
  extends the shapes and keeps every check.
- [`native-refresh-scoped`](native-refresh-scoped.md): a lean wrapper fix
  validated by the next scheduled run, which this design follows for its
  native validation.
- [`componentization`](componentization.md) (not in `related`: this is a
  process item, not a program item): the phases whose native work needs
  these fixes (K1 to K4, M2, M3).

## Decisions

1. **One item, eleven fixes, one review** (Jeff's word). The fixes share
   files (the wrapper takes fixes 2, 6, 7, 8 and 10) and are each small.
   Alternative: an item per defect, which is the churn Jeff ruled out.
2. **A run is "worked" only when its digest reached the workflow branch**
   (fixes 1, 2). The skill already makes the digest the run's last act, so
   its presence on `origin/workflow` is the one observable sign that the
   session finished. Alternatives: trust `claude`'s exit code (today's
   bug); watch for background processes alone (misses a session that just
   stopped early, and cannot see processes under Git Bash).
3. **The session waits for its own background work, and the wrapper is the
   backstop** (fix 1). The skill rule stops the cause; the wrapper's check
   (Decision 2) and its sweep of processes the session left catch what
   the rule misses. Alternative: forbid background commands, which would
   make every case longer than the Bash tool's 10-minute ceiling
   unrunnable unattended.
4. **A saved leftover reaches the next session inside the checkout** (fix
   2), as `.scratch/leftover/<run>/`, named in its prompt. The session
   cannot read `$state`. Alternative: leave it for an attended session,
   which is what cost Jeff the recovery on 2026-11-23.
5. **Branch setup takes its base from `wf trunk`, in each repository**
   (fixes 3, 4). It is the one reader of the item's trunk (FLOW-20);
   alternative: name `program/componentization` in the skill, which goes
   stale when the program lands.
6. **`native-run.sh` accepts a second repository's worktree with the same
   checks** (fix 5): a tracked file of a worktree whose common directory
   is the clone's at `.dev/checkouts/<owner>/<repo>`, outside its
   `tools/workflow/`. Alternative: scratch Makefiles only, which the skill
   already counts as an allowlist gap under **Blocked on Jeff** each time:
   an interaction that decides nothing.
7. **The fingerprint adds each repository's program trunk** (fix 6), and
   FLOW-09's sentence says so. Alternative: rely on the 7-day sweep, which
   delays a program trunk's native validation up to a week.
8. **Cleanup tests a branch against the item's own trunk, by `git branch
   -d` with that trunk as its upstream** (fix 7), keeping git's safe form.
   Alternative: `merge-base --is-ancestor` then `branch -D`, which FLOW-16
   allows only for a squash merge after `wf check`.
9. **The Entire hooks are regenerated by Entire, not by hand** (fix 9):
   `entire agent add claude-code` at Implement, with the version recorded.
10. **Pipelines into `grep -q` read a captured variable instead** (fix 10),
    in every native script, not only the two lines found.
11. **The macbook's host checks are profile entries, argv only** (fix 11),
    with `sh -c` where a check needs a pipe; the Entire hook check is in
    `wf native -check`, for the dedicated checkout and every clone.

## Questions

### Blocking

None.

### Assumed

- **a1. How an unattended session waits on a case longer than the Bash
  tool's 10-minute ceiling.** Proceeding with (A): a command that fits
  the ceiling runs in the foreground; a longer one runs in the background
  and the session waits for it in repeated bounded waits until it exits,
  or stops it and records the case as not run, before it writes its
  digest. If the wait tool the session needs is one
  `settings.native.json` does not allow, that is an allowlist gap under
  **Blocked on Jeff**, and this item adds no permission. Options: (A) as
  stated; (B) foreground only, long cases split into scratch Makefile
  targets each under 10 minutes (multi-hour installs cannot split);
  (C) the wrapper runs long cases itself (a second driver of validation,
  outside the skill). Recommended: (A).
- **a2. Flagged: FLOW-16 is a rule FLOW-20 reserves to Jeff (pushing).**
  Whether a program item's local branch is "contained in trunk" when its
  program trunk contains it. Proceeding with (A): yes, by FLOW-20, which
  makes the program trunk the item's trunk for its merge; no FLOW-16
  wording change; the wrapper sets the upstream to `origin/<wf trunk>` and
  runs `git branch -d`. Options: (A) as stated; (B) amend FLOW-16 to say
  "its trunk (for a program item, its program trunk, FLOW-20)", a second
  rule change in this review; (C) keep testing against `main`, with the
  nightly note. Recommended: (A).
- **a3. Flagged: it widens the shapes of the unattended launcher
  (`native-run.sh`).** Proceeding with (A): accept
  `.scratch/<owner>/<repo>/<item>/tools/<path>`,
  `.scratch/<owner>/<repo>/<item>/dist/warden*` and `go -C
  .scratch/<owner>/<repo>/<item> test`, only where `<owner>/<repo>` has a
  clone at `.dev/checkouts/<owner>/<repo>` and the worktree's common
  directory is that clone's; every other check stays. No allow rule
  changes. Options: (A) as stated; (B) scratch Makefiles only, each an
  allowlist gap for Jeff; (C) a separate launcher for second
  repositories. Recommended: (A).
- **a4. Non-worktree directories under `.scratch/`.** Proceeding with
  (A): each is named in the leftovers; one whose directory has not
  changed for `BW_SCRATCH_DAYS` (default 7) is removed. Excluded: the
  `<owner>` directories of listed repositories (they hold the clones'
  worktrees) and `.scratch/leftover/` (fix 2). Options: (A) as stated;
  (B) named only (the nightly note Jeff found); (C) removed at the next
  run (a crashed session's scratch logs are lost before anyone reads
  them). Recommended: (A).
- **a5. Native validation tracked, not blocking.** Proceeding with (A):
  `macos-any` and `windows-any` tracked, with backlog entries for the
  first scheduled runs after the merge, as `native-loop-followups` and
  `native-refresh-scoped` did. Options: (A) tracked; (B) a blocking
  `f6-validate-macos-any` step, run attended from a branch worktree (the
  wrapper resets its own checkout to `main`, so only the install check,
  `wf native -check`, the launcher and the gate could be run from the
  branch). Recommended: (A).
- **a6. A missing Entire git hook fails `wf native -check`.** Proceeding
  with (A): FAIL, with the command to fix it, since a commit in a clone
  without them is not linked to its session (FLOW-18). Options: (A) FAIL;
  (B) warn. Recommended: (A).

## Design

Everything is in this repository (`repos` empty); a second repository's
clone is reached only by the wrapper's existing reset and the skill's
existing commands.

### 1. The session finishes its background work

`.claude/skills/workflow-native/SKILL.md`, **Never (unattended)**, gains:

> never end the run while a command this session started in the
> background is still running: a command that fits the Bash tool's
> timeout runs in the foreground; a longer one is waited for, in bounded
> waits, until it exits, or stopped and its case recorded as not run
> (release the step, or keep the backlog entry open, with the reason),
> before the digest is written.

**Close (unattended)** names the digest file by the run part of the
session id, `<run>-host-<machine>.md` (what the runs already do: e.g.
`20261122T124005Z-host-macbook.md`), so the wrapper can find it.

### 2. The wrapper checks that the session finished, and hands on what it left

`native-tick.sh`, after `claude` exits:

1. **Sweep.** The wrapper exports `BW_NATIVE_SESSION=$machine.$run` to the
   session. After it exits, it sends TERM to each process of the user that
   still carries it (`ps -axeww -o pid=,command=` on macOS, which shows the
   environment; `/proc/*/environ` on Linux; skipped under Git Bash, whose
   `ps` shows neither), and notes "killed N background processes the
   session left".
2. **Finished?** It fetches `origin/workflow` and looks for
   `digest/*/<run>-host-<machine>.md`. Missing, or a sweep that killed
   anything, or leftover workflow changes (`reset_workflow`, moved here
   from `on_exit`), makes the outcome `failed`, with the reason "session
   ended unfinished: <what>".
3. **Stamp.** `gate-<machine>.json.pending` moves to the stamp only when
   the outcome is `worked`. Otherwise it is removed, so the next run's gate
   compares against the last finished run.
4. **Hand-on.** A saved leftover (`$state/leftover/<run>/`) is copied at
   the next run, after the sync and before the gate, to
   `.scratch/leftover/<run>/`, and the prompt gains "Unfinished: run <run>
   ended unfinished; its saved workflow changes are in
   `.scratch/leftover/<run>/`." A hand-on starts no run by itself
   (FLOW-09's triggers are unchanged); until a run with work takes it, the
   status reason names it ("unfinished run <run> not yet handed on"). The
   steps an unfinished run held are released by the wrapper and so are
   ready, which is such a trigger. The skill's **Arguments** says:
   read them, record the evidence they hold on their steps and backlog
   entries as `wf done`, `wf release` or a backlog note (as the attended
   session of 2026-11-23 did by hand), and name the run under **Notes**.
   The wrapper deletes `.scratch/leftover/<run>/` and marks
   `$state/leftover/<run>/handed` once a run that was told of it ends
   `worked`.

### 3. Branch setup from the item's trunk

SKILL.md **Work**: at Branch setup, run `dist/wf trunk -state .dev/workflow
-feature <item>` first and use its first field literally (no command
substitution, which the allowlist matcher cannot see through):
`git worktree add .scratch/<item> -b feat/<item> origin/<trunk>`.

### 4. A second repository's Branch setup, and a kept worktree

SKILL.md **Work**, for a second repository, after `git -C
.dev/checkouts/<owner>/<repo> fetch origin`:

- Branch setup: `dist/wf trunk ... -feature <item> -repository
  <owner>/<repo>`, then `git -C .dev/checkouts/<owner>/<repo> worktree add
  -b feat/<item> ../../../../.scratch/<owner>/<repo>/<item>
  origin/<trunk>`.
- Later steps, no worktree yet: as today (`worktree add ... feat/<item>`).
- A worktree kept from an earlier night (the wrapper keeps it while the
  item is open): no `worktree add`; `git -C
  .scratch/<owner>/<repo>/<item> merge --ff-only origin/feat/<item>`, which
  `Bash(git -C .scratch/*)` already allows. A refused fast-forward (local
  commits not pushed) is pushed first with `native-push.sh`, or the step
  released with the reason.

The same kept-worktree form is added for this repository's
`.scratch/<item>`. No allow rule changes.

### 5. `native-run.sh` takes a second repository's worktree

The `.scratch/*/tools/*` case splits the path at the first `/tools/` or
`/dist/` and accepts a worktree path of one component (`.scratch/<name>`,
this repository's, as today) or three (`.scratch/<owner>/<repo>/<item>`).
For three components, each is a `valid_name`; `.dev/checkouts/<owner>/<repo>`
must be a git checkout; the worktree must be its own top level and its
common directory that clone's. The tracked-file, no-`.`/`..`, and
not-`tools/workflow/` checks apply unchanged. `dist/warden*` and `go -C
<dir> test` accept the same worktree paths. The usage line and header
comment list the new shapes.

### 6. The gate wakes on a program trunk's move

`queue/hosts.go` `fingerprint`: after `refs/remotes/origin/<trunk>`, append
`refs/remotes/origin/<program_trunk>` when `Config.ProgramTrunkOf(name)` is
set. An absent ref hashes as `absent`, as now. The gaps reason reads "a
trunk, a program trunk or a waiting branch moved". The doc comment drops
"a14's" trunk-only wording. Every machine's stamp differs once after the
deploy: one extra sweep, which is the sweep a program-trunk move would
have earned anyway.

### 7. Cleanup follows the item's trunk and names what it skips

`native-tick.sh` builds `dist/wf` before the worktree cleanup (today it is
built after). In `clean_worktrees`:

- **A feature worktree** whose item is done or whose branch is gone: after
  `git worktree remove`, it reads the item's trunk (`dist/wf trunk -state
  .dev/workflow -feature <item> [-repository <owner>/<repo>]`), and when it
  is not the clone's trunk, sets it as the branch's upstream (`git branch
  --set-upstream-to=origin/<trunk>`) before `git branch -d`, which then
  tests containment in the program trunk. A squash-merged process item is
  still refused and noted, as `loop-native-lead` decided. `wf trunk`
  failing (no queue file) keeps today's test, named in the note.
- **A detached worktree** (validation of a ref): removed with `git
  worktree remove` when clean, which loses nothing; noted when dirty.
- **A non-worktree directory**: noted, and removed when older than
  `BW_SCRATCH_DAYS` (a4), except the listed repositories' `<owner>`
  directories and `.scratch/leftover/`.

### 8. PATH

`native-tick.sh:55` and `install-macos.sh:22` add
`/opt/homebrew/opt/rustup/bin` and `$HOME/.cargo/bin`, after
`~/.local/bin` and before Homebrew's `bin`, so rustup's proxies win over a
Homebrew `rust` formula. The macbook's `~/.local/bin` links can then go
(Jeff's, outside the checkout).

### 9. Entire hooks

At Implement, with Entire CLI 0.12.2, `entire agent add claude-code` in the
worktree; commit only `.claude/settings.json`'s change (the SessionStart
line, as reproduced). The commit message names the Entire version. The
macbook's own Entire must be the same version, or the drift reappears;
the install check prints `entire --version`.

### 10. No `| grep -q` under pipefail

`install-macos.sh:100`, `:135` and `native-tick.sh:286` capture the
command's output first (`batt=$(pmset -g batt)`) and test it with `case`.
A test in `native_test.go` greps the native scripts for `| *grep -[a-z]*q`
and fails on a match, so the shape does not return.

### 11. Host checks and the Entire git hooks

`docs/development/loop-profile.yaml` `native.check` gains:

```yaml
    macos-finch: [[sh, -c, "finch vm status | grep -qx Running"]]
    macos-qemu: [[qemu-system-aarch64, --version]]
    qemu-any: [[qemu-system-aarch64, --version]]
    macos-vz: [[sh, -c, "codesign -d --entitlements - dist/warden 2>&1 | grep -q com.apple.security.virtualization"]]
```

(`sh` has no `pipefail`, so these pipes are safe; a failed `native.check`
warns, as today.) PROFILE-01's Native hosts row says so.
`cmd/wf/native.go` `nativeCheck` gains, for the dedicated checkout and each
present clone, a check that `git rev-parse --git-path hooks` holds
`prepare-commit-msg`, `commit-msg`, `post-commit`, `post-rewrite` and
`pre-push`, each naming `entire`; a miss is a FAIL naming `entire enable`
in that directory (a6). The five names are what Entire 0.12.2 installs.

### Every rule, machinery path and configuration key the change reaches

- **Rules changed:** FLOW-09 (the gate's trigger sentence), PROFILE-01
  (the Native hosts row and the profile file). See **Rule changes**.
- **Rules relied on, unchanged:** FLOW-04 (leases), FLOW-13 (native
  budget), FLOW-16 (cleanup's safe forms; a2), FLOW-17 (pushes only by
  `native-push.sh` and `wf commit`), FLOW-18 (recording), FLOW-20 (the
  item's trunk), LOOP-05 (clones are the owner's), LOOP-09, FLOW-14.
- **Machinery paths:** `.claude/skills/workflow-native/SKILL.md`;
  `.claude/settings.json` (Entire's SessionStart hook line only);
  `tools/workflow/native/native-tick.sh`, `native-run.sh`,
  `install-macos.sh`, `native_test.go`; `tools/workflow/queue/hosts.go`
  and its test; `tools/workflow/cmd/wf/native.go` and its test.
- **Rule paths:** `docs/development/process.md`,
  `docs/development/loop-profile.md`, `docs/development/loop-profile.yaml`.
- **Configuration keys:** `loop-profile.yaml` `native.check.{macos-finch,
  macos-qemu, qemu-any, macos-vz}` (added). Environment:
  `BW_SCRATCH_DAYS` (new, wrapper), `BW_NATIVE_SESSION` (new, exported to
  the session), the wrapper's and install script's `PATH`. The gate stamp
  `gate-<machine>.json` (moves on `worked` only).
- **Not reached:** `config.yaml` (no limit, budget, lease or
  `runaway_factor`), `hosts.yaml`, `settings.native.json` and every
  `.claude/settings*.json` allow or deny list, `native-push.sh`,
  `install-windows.ps1`, every merge, push and tag path.

## Program payoff

Figures from the workflow branch's `native/status/macbook.yaml` history,
the backlog notes, the componentization phase table and the queue
estimate:

- **Lost nights and their recovery.** Of the 7 scheduled macbook runs
  2026-11-17..23, 2 (11-20, 11-23) ended unfinished (fixes 1, 2); each
  lost that night's evidence and cost Jeff an attended recovery that
  decided nothing (on 11-23, recording the saved leftover by hand). At
  that rate, about **2 human interactions a week** removed, and the
  nights kept.
- **A nightly note that decides nothing.** Once a program item's branch
  is gone on the macbook, fix 7's missing case writes "branch ... not
  deleted (FLOW-16)" in every run's status and digest: **7 notes a week**
  per such branch for Jeff to read and dismiss. Detached worktrees and
  scratch directories accumulate unnamed until an attended cleanup.
- **Program time.** The macbook serves the vz, qemu and container drivers
  and lists the relay and three driver repositories. Phases K1, K2, K4,
  M2 and M3 put native steps there; counting one item per driver per
  phase that touches it (K1 container; K2 vz, qemu, container; K4 vz,
  qemu, container; M2/M3 the relay on vz and qemu) gives **at least 9
  items**. Without fixes 3 to 5 and 8, each second-repository or
  program-trunk item's native steps need an attended macbook sitting
  (wrong base, no kept-worktree form, launcher refusals, no rustup): up
  to 9 sittings over the program. Without fix 6, each program-trunk merge
  whose only native work is backlog waits up to `BW_SWEEP_DAYS` = 7 days
  for its sweep instead of one night.
- **Cost.** The queue estimate: 6 steps over 2 runs, and one review
  sitting with Jeff (this design's review is required, below).

## Defaults and overrides

- **The stamp moves only on a finished run** (digest present, no leftover,
  nothing swept). Obvious: the stamp means "what the last full session
  saw" (`native-host-loop`). Override: `BW_FORCE=1` skips the gate as
  today; nothing else.
- **`BW_SCRATCH_DAYS`, default 7**, the age past which an unnamed scratch
  directory goes; the same week as `BW_SWEEP_DAYS`, so a directory
  outlives at least one sweep. Override: the variable, exclusively.
  Settled by a4.
- **PATH order:** `~/.local/bin` first (the owner's own overrides), then
  rustup's proxies, then Homebrew. Override: a link in `~/.local/bin`.
- **Named fallbacks:**
  - *Digest check cannot fetch `origin/workflow`*: the run is `failed`
    ("cannot confirm the session finished"), fail closed; the status file
    shows it.
  - *Process sweep under Git Bash*: skipped (no environment in `ps`);
    the digest check alone decides there; the reason says "sweep
    skipped".
  - *`wf trunk` fails at cleanup*: `branch -d` against the clone's trunk,
    as today, and the note says the item's trunk was not read.
  - *Kept worktree refuses `merge --ff-only`*: push it first or release
    the step with the reason; never a reset or a force-push (FLOW-17).

## Cross-driver impact

`drivers` is empty: no driver code changes. The `macos-vz` check reads
`dist/warden`'s entitlement; the `macos-finch` and qemu checks read the
host's tools. Hyper-V's `native.check` and `native.prepare` are unchanged.

## Rule changes

### FLOW-09 (amend), `docs/development/process.md`

Current, the sentence in the third paragraph:

> Each native machine also runs a scheduled session that starts an agent
> only when a host step it serves is ready, or when `main` or a waiting
> branch has moved since its last sweep of open backlog entries, and
> never while the human has not confirmed the priority order;

Proposed:

> Each native machine also runs a scheduled session that starts an agent
> only when a host step it serves is ready, or when a trunk, a program
> trunk or a waiting branch of a repository it lists has moved since its
> last finished sweep of open backlog entries, and never while the human
> has not confirmed the priority order;

Severity MUST and check `automated + agent-judgment`, before and after.
Why: FLOW-20 makes the program trunks where program items merge, and the
skill already tests them; the gate's trigger lagged. "Finished" records
fix 2: an unfinished run does not count as a sweep. Enforcement:
`queue/hosts.go`'s fingerprint and the wrapper's stamp, with their tests.

### PROFILE-01 (amend), `docs/development/loop-profile.md` and `loop-profile.yaml`

Current, the Native hosts row's entry:

> The host names in the loop's `hosts.yaml`. All of them serve this
> repository's drivers or its platform builds today. Before a session on a
> machine that serves `windows-hyperv`, the native wrapper runs
> `tools/relay-vm/hyperv/refresh-cache.sh` (the profile file's
> `native.prepare`), and the install script runs it with `--check` (its
> `native.check`).

Proposed:

> The host names in the loop's `hosts.yaml`. All of them serve this
> repository's drivers or its platform builds today. Before a session on a
> machine that serves `windows-hyperv`, the native wrapper runs
> `tools/relay-vm/hyperv/refresh-cache.sh` (the profile file's
> `native.prepare`), and the install script runs it with `--check` (its
> `native.check`). The install script also checks, by `native.check`,
> that finch's VM is running (`macos-finch`), that `qemu-system-aarch64`
> runs (`macos-qemu`, `qemu-any`) and that `dist/warden` carries the
> virtualization entitlement (`macos-vz`).

The profile file gains the four entries in **Design**, item 11. Severity
MUST and check `agent-judgment`, before and after. Why: the macbook's
install check passed with finch's VM stopped and an unsigned warden, each
of which fails that night's steps. Enforcement: none changes; `wf native
-check` already runs `native.check` (warn on failure).

## Validation plan

- **Tier 0** (`make test`, `make lint`, in `tools/workflow`):
  - `hosts_test.go`: the fingerprint changes when only a program trunk's
    ref moves, and not when an unlisted repository's does.
  - `native_test.go` (launcher): accepted
    `.scratch/<o>/<r>/<i>/tools/x.sh`, `.../dist/warden`, `go -C
    .scratch/<o>/<r>/<i> test`; refused: no clone at `.dev/checkouts/<o>/<r>`,
    a worktree of another repository, an untracked file, a `tools/workflow/`
    path, a `..` component, two components.
  - `native_test.go`: no `| grep -q` shape in the native scripts.
  - `cmd/wf/native_test.go`: the Entire hook check, ok and FAIL.
  - `bash -n` and shellcheck on `native-tick.sh` and `install-macos.sh`;
    the wrapper's stamp, digest and cleanup paths against a scratch bare
    remote, as `native-loop-followups` tested its trap.
- **Tier 2, tracked** (a5): `macos-any` and `windows-any`, backlog
  entries `native-loop-fixes-macbook` and `native-loop-fixes-windows`:
  the first scheduled runs after the merge (a run with background work
  left, a program-trunk-only move, a gone program branch's cleanup, a
  detached worktree, the install check with finch stopped and with an
  unsigned warden).

## Implementation plan

1. Skill: **Never (unattended)**, **Arguments** (Unfinished), **Work**
   (Branch setup and kept-worktree forms, both repositories), **Close**
   (digest name) (FLOW-14; fixes 1 to 4).
2. `native-tick.sh`: sweep, digest check, stamp, hand-on, build `wf`
   before cleanup, cleanup by the item's trunk, detached and scratch
   directories, PATH, `pmset` (FLOW-09, FLOW-16, FLOW-17; fixes 2, 7, 8,
   10).
3. `native-run.sh` and its cases (fix 5).
4. `queue/hosts.go` fingerprint and test (fix 6).
5. `install-macos.sh`: PATH, the two `grep -q` lines, print `entire
   --version` (fixes 8, 9, 10).
6. `cmd/wf/native.go`: the Entire hook check (fix 11).
7. Rule changes once approved: FLOW-09 in `process.md`; PROFILE-01's row
   and `loop-profile.yaml` (FLOW-14).
8. `.claude/settings.json` by `entire agent add claude-code` (fix 9).
9. `make test`, `make lint`; push (FLOW-17).

Done, x2-implement, 2026-11-23, one commit per task:

1. 572f687: the skill's **Never (unattended)** background rule,
   **Arguments**' Unfinished line, **Work**'s Branch setup from `wf trunk`,
   later-step and kept-worktree forms for both repositories (with
   `native-run.sh` named for a second repository's scripts), and
   **Close**'s `<run>-host-<machine>.md`.
2. b320471: `native-tick.sh`. The sweep, digest check and leftover save
   run after `claude` exits (`check_finished`); the stamp moves on `worked`
   only and is removed otherwise; `hand_on` and `handed_off`; `dist/wf` and
   the clone list before `clean_worktrees`, which tests a branch against
   `wf trunk`'s trunk, removes clean detached worktrees and names or ages
   out non-worktree directories; PATH; the `pmset` capture. Two additions
   beyond **Design**: the EXIT trap keeps `reset_workflow` as a fallback for
   a run that ended before the post-session check (so a leftover is never
   reset away), and a non-worktree directory that holds a `.git` is named
   but never removed. One defect found and fixed in passing: `fetch`'s bare
   `return`, called from the EXIT trap, returned the script's exit status
   (bash 4+), so a failed run could never push its status. Tested against
   a scratch bare remote with a fake `claude`: finished, background work
   left (two processes killed), no digest, stray workflow changes (saved,
   then handed on and marked by the next worked run), and a gated idle
   run's cleanup (a program item's gone branch deleted against
   `origin/program/componentization`; an unmerged branch with no queue
   file named; clean and dirty detached worktrees; old, new and
   `.git`-holding scratch directories).
3. 44485f9: `native-run.sh` and `native_test.go`'s accepted and refused
   second-repository cases.
4. ce79895: `queue/hosts.go` and `TestFingerprintProgramTrunk`.
5. 035666e: `install-macos.sh` and `TestNoPipeIntoGrepQ`. The test's
   pattern is `\| *grep +-[a-zA-Z]*q`, so it also catches `-Eq`
   (`install-macos.sh:100`), which **Design**'s `[a-z]*` would miss.
6. f7d9a10: `wf native -check`'s Entire hook check and
   `TestNativeCheckEntireHooks`.
7. 03ed9cb: FLOW-09 and PROFILE-01, in the approved wording.
8. 095c42d: `entire agent add claude-code` with Entire CLI 0.12.2; only
   the SessionStart line changed.
9. a89fda2 (line wraps for `lll`), c5131a9 (puts back the
   `tools/workflow/cmd/wf/wf` binary `main` tracks, which a local build
   had overwritten in f7d9a10). `make test` and `make lint` green.

## Design review

- r1, 2026-11-23: approved at a5aed35. Jeff approved the design as read at
  a5aed35, in the attended session.
  - The assumed questions a1 to a6 are answered (A), each the
    recommendation. Jeff: they read reasonable, and they are re-evaluated
    later if found incorrect.
  - Each rule change was asked on its own:
    - rule change FLOW-09 approved at a5aed35
    - rule change PROFILE-01 approved at a5aed35
  - Nothing was folded in.

## Consolidation

## Rollback notes

## Archive summary
