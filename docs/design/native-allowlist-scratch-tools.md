---
feature: native-allowlist-scratch-tools
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-05, FLOW-08, FLOW-09, FLOW-10, FLOW-12, FLOW-13, FLOW-14, FLOW-15, FLOW-16, FLOW-17, FLOW-18, FLOW-19, TEST-01, TEST-02, UX-01, GLOBAL-01]
rule_changes: []
concepts: [native-session, permissions, workflow, scratch-worktree, env-prefix]
related: [native-allowlist-scratch.md, native-host-loop.md, native-loop-followups.md]
---

# Native allowlist: a branch's tools and WARDEN_* variables

## Problem

The unattended native session runs under
`tools/workflow/native/settings.native.json` (passed by
`tools/workflow/native/native-tick.sh:249`). It allows a branch's built CLI as
`.scratch/*/dist/warden*` (line 29) and the repository's own workflow scripts
as `tools/workflow/*` (line 33), but nothing else under a scratch worktree, and
no command that starts with a variable assignment.

windows-desktop's run `host:windows-desktop/20261105T051002Z`
(`digest/2026-11-04.md:334` on the `workflow` branch, **Blocked on Jeff →
Native allowlist gap**, lines 406-411) validated build-link-subnet at
`feat/build-link-subnet` `a68ac2c`. Two of its windows-hyperv cases needed
commands the file refuses:

- the branch's `tools/relay-vm/hyperv/refresh-cache.sh` (and `--check`), run
  from `.scratch/build-link-subnet`, to install the branch-built relay boot
  VHDX and its link-channel stamp;
- the forged-stamp case, `WARDEN_RELAY_BOOT_VHDX=<old disk>
  .scratch/build-link-subnet/dist/warden.exe build --driver hyperv ...`.
  `driver/hyperv/buildassets.go:42` reads that variable; no flag sets it.

The session wrote `native/windows-hyperv/2026-11-04/scripts/Makefile`
(committed in `e7b8738`) with `refresh`, `check` and `forged` targets and ran
them as `make -C .scratch/hv <target>`, which `Bash(make:*)` allows. The
validation passed, but the evidence now depends on a hand-written launcher
whose only job was to get round the allowlist, and the next session will have
to invent it again.

The cause of the second gap is the matcher, not a missing rule. The Claude
Code permissions reference ("Wrappers") says it strips a leading assignment
of "certain known-safe environment variables" before matching, and "an allow
rule won't match past an assignment of any other variable"; `WARDEN_*` are not
on that built-in, non-configurable list. A deny rule matches past any
assignment.

## Prior art

- `docs/design/native-allowlist-scratch.md` (archived; `adbe406` is the
  settings file's last change): replaced the `prefix/:*` rules with
  mid-pattern wildcards and added `.scratch/*/dist/warden*`. Its prior art
  (lines 44-47) records that `*` stands for any text anywhere in the rule,
  spaces and slashes included. It rejected `Bash(.scratch/*)` because it
  "would run any file a branch put under `.scratch/`, not only the CLI it
  built". Its archive summary: these rules "filter command text and are not a
  security boundary, branch protection on `main` is". The existing
  `Bash(.scratch/*/dist/warden*)` already admits any `.scratch/` command with
  `/dist/warden` somewhere after it; this item adds no rule of that shape.
- `docs/design/native-host-loop.md:94-102` (archived): the settings file is
  "a guard against accidents, not a sandbox: `make`, `go` and `powershell` can
  run anything. The dedicated checkout, the skill's Never list and FLOW-08 are
  the real boundary."
- `docs/design/native-loop-followups.md:170-176, 279-281` (archived): Jeff
  approved scratch-worktree rules on 2026-10-29 because "`make:*` already lets
  the agent run anything".
- `.claude/skills/workflow-native/SKILL.md:37-48` names the forms a session
  uses on a branch: `git -C .scratch/<name>`, `.scratch/<name>/dist/warden`,
  `make -C .scratch/<name> <target>`. Its **Never** lines (25-35) forbid
  edits to `tools/workflow/` and changes outside the checkout and caches. Its
  **Record** section (84-99) covers a step that passes, a failure and a
  later-phase refusal, but not a case the allowlist refused.
- FLOW-14: `.claude/` and `tools/workflow/` outside `check/` change only
  through a process item, and reach the running loop only through the human's
  merge (FLOW-08). `native-tick.sh` runs from the `main` checkout, so the
  settings file and anything under `tools/workflow/` that the session runs are
  `main`'s.
- FLOW-05 and the `workflow` branch's `README.md:108-113`: while a process
  item is `in_progress`, a worker matching its `holds` takes no new step and
  `wf gate` starts no sweep for a held host (`tools/workflow/queue/next.go:34-46`
  checks `Status == "in_progress"`).
- `tools/relay-vm/hyperv/refresh-cache.sh:23-24, 33` resolves the repository
  from its own path (`$here/../../..`) and `cd`s there, so invoking a
  worktree's copy by path from the main checkout root builds that worktree's
  relay. It is tracked with mode `100755`.
- No tier-0 test reads `settings.native.json` today (`grep` over
  `tools/`, `Makefile`, `.github/`: only `native-tick.sh` and
  `flow18-check.sh` name settings files).

## Decisions

1. **No new allow rule: a branch's scripts and `WARDEN_*` variables both go
   through one main-owned launcher, `tools/workflow/native/native-run.sh`.**
   It is already allowed by `Bash(tools/workflow/*)` (line 33), so
   `settings.native.json` does not change. Usage:
   `tools/workflow/native/native-run.sh [NAME=value ...] -- <command> [args]`.
   With no assignment it runs a branch's script; with assignments it also
   sets variables (Decision 2). For a branch script,
   `.scratch/<name>/tools/<path>`, it checks before running anything:
   - `<name>` is one path component, and `<path>` has no empty, `.` or `..`
     component;
   - `<path>` does not start with `workflow/` (a branch's loop tooling is not
     run by the session; under FLOW-14 it reaches the loop only by merge);
   - `.scratch/<name>` is a worktree of this repository
     (`git -C .scratch/<name> rev-parse --git-common-dir` is the main
     checkout's `.git`), and `git -C .scratch/<name> ls-files --error-unmatch
     tools/<path>` succeeds, so the target is a tracked file of a branch, not
     a file the session wrote.
   Rejected:
   - `Bash(.scratch/*/tools/*)` with a deny on
     `Bash(.scratch/*/tools/workflow/*)` (this design's first draft). The
     first `*` matches any text, so the allow admits any command that starts
     with `.scratch/` and contains `/tools/` anywhere: `.scratch/hv/tools/run.sh`
     written by the session itself (`.scratch/hv` is the incident's own
     non-worktree scratch directory), `.scratch/x/tools/../../../bin/bash -c ...`,
     `.scratch/evil.sh foo/tools/bar`. That is close to the `Bash(.scratch/*)`
     the archived design rejected, and it is exactly the unprompted path to a
     workaround script that Decision 3 forbids. The deny is a text filter too:
     `tools/./workflow/` and `tools//workflow/` miss it, and it over-matches a
     `warden build` whose arguments contain `/tools/workflow/`.
   - `Bash(.scratch/*/tools/relay-vm/*)`: the same `*` width, and the next
     host-specific script (`tools/relay-vm/build-initramfs.sh`, a future
     `tools/<driver>/`) reopens the gap.
   - `Bash(.scratch/*)`, for the reason the archived design gave.
   - Two launchers, one for scripts and one for variables: the same checks
     twice, and a branch script that needs a variable would need both.
2. **`WARDEN_*` variables go through the launcher, not an assignment
   prefix.** `native-run.sh` takes assignments up to `--`; each name must
   match `WARDEN_[A-Z0-9_]+` and must not end in `_PASSWORD`, `_TOKEN`,
   `_SECRET` or `_KEY` (so `WARDEN_QEMU_VNC_PASSWORD` is refused: a secret on
   a command line is in the transcript, FLOW-18). With at least one
   assignment it also accepts, with no `..` path component,
   `.scratch/<name>/dist/warden*`, `dist/warden*`, `./warden*`, `go test ...`
   or `go -C .scratch/<name> test ...`. It exports the assignments and
   `exec`s the command, so its exit status and output are the command's, and
   refuses anything else with a one-line message on stderr naming the
   accepted shapes, exit 2, before running anything.
   Rejected:
   - Allow rules that spell the assignment, such as
     `Bash(WARDEN_RELAY_BOOT_VHDX=* .scratch/*/dist/warden*)`: the reference
     documents that an allow rule does not match past an unknown assignment
     and does not say whether a rule containing the assignment matches it;
     the mid-pattern `*` also matches spaces, so the rule would read wider
     than it looks. Relying on undocumented matcher behaviour is the bug the
     archived design fixed.
   - `Bash(env *)` (or `env --`): `env` runs any command. `go:*`, `make:*`
     and `powershell:*` already step round every deny, so this is not a new
     capability; it is rejected because it gives the digest no documented
     form and makes the refusals the launcher enforces (secrets, a branch's
     `tools/workflow/`, untracked files) unenforceable.
   - `powershell -Command "$env:WARDEN_RELAY_BOOT_VHDX='...'; .scratch\x\dist\warden.exe ..."`,
     which `Bash(powershell:*)` already admits on windows-desktop: it hides
     the real command inside a string, like the Makefile, and has no macOS
     equivalent.
   - A `--relay-boot-vhdx` flag on `warden`: a product change (UX-01, a
     config surface) made for test plumbing, and it covers one variable of
     the eighteen `WARDEN_*` names non-test code reads, besides test-only ones
     such as `WARDEN_HYPERV_LIVE` (`WARDEN_HYPERV_CACHE_DIR`,
     `WARDEN_HYPERV_BUILD_IMAGE`, `WARDEN_QEMU_MONITOR`, ...).
   - A named set of variables instead of the `WARDEN_*` pattern: every new
     variable a test needs reopens the gap, and the branch-built binary that
     reads them is the branch's code anyway (Security note).
   - A launcher accepting any command: the same as `env`.
3. **The skill names the launcher first, sanctions the scratch Makefile
   for what the launcher does not cover, and says how to record both** (a4,
   amended in review r1: Jeff chose to allow the Makefile pattern).
   `workflow-native` step "Git on the workflow checkout" gains the launcher
   forms, and says: when a command the procedure needs is outside the
   launcher's forms and still refused, put it in a scratch Makefile under
   `.scratch/<name>/` and run it with `make -C .scratch/<name> <target>`
   (already allowed). The digest names each such target and the exact
   commands its recipe runs, so the real command stays visible to the reader,
   and lists it under **Blocked on Jeff** as an allowlist gap to consider.
   **Record** gains the queue handling for a case neither form can run: the
   step is released with the refused command as the reason (not `wf done`)
   and stays pending; a backlog entry stays open with the same note. Scratch
   test inputs (a build script, a probe program, a test context) stay fine.
   Rejected:
   - Forbidding the scratch Makefile (the draft's choice): Jeff preferred
     the case to run and the gap to be reported over the step stopping.
   - Skipping the case and finishing the step: a blocking FLOW-09 step would
     be marked done with a case never run.
   - Saying nothing: the next refused command produces the next Makefile.
4. **A tier-0 test guards the file and the launcher.** A new test-only
   package `tools/workflow/native` (`native_test.go`), run by `make test`
   (`cd tools/workflow && go test ./...`, `Makefile:35`):
   - `TestNativeSettingsRules`: `settings.native.json` parses; it holds
     `Bash(tools/workflow/*)` (the launcher's only route); no allow rule ends
     in `/:*` (the archived design's bug class); every
     `Bash(git -C * push ...)` deny of the archived Decision 2 is present; no
     allow rule starts with `Bash(.scratch/*/tools`.
   - `TestNativeRun` (run with `sh`, skipped where `sh` or `git` is missing):
     a temp repository with a worktree `.scratch/t` holding a tracked
     `tools/x.sh` and a stub `dist/warden` that print their environment, plus
     an untracked `tools/y.sh` and a tracked `tools/workflow/z.sh`. Accepted:
     `-- .scratch/t/tools/x.sh` and `WARDEN_A=1 -- .scratch/t/dist/warden`,
     with the variable and the exit status passed through. Refused, exit 2,
     nothing run: a non-`WARDEN_` name, `WARDEN_X_PASSWORD`, no `--`, a
     command outside the shapes, `..`, `.` and empty components, the
     untracked `tools/y.sh`, `tools/workflow/z.sh`, a `.scratch/<name>` that
     is a plain directory, and `dist/warden` with no assignment.
   Rejected: shellcheck only (the repository runs none in `make lint`), or
   no test (the archived bug lived because nothing read the file).
5. **The `host:*` hold stays (a6).** The native session reads `main`'s
   settings and skill, so nothing on this branch can disturb a native run;
   the hold's only effect is that no native run meets the gap, and writes
   another workaround, before the skill text merges. Its cost today is small:
   no queued or in-progress item has a pending `host:` step (checked
   2026-11-06; items 43-46 are `queued` with none), and the three open
   native-gap backlog entries were skipped on 2026-11-06 because `main` had
   not moved.
6. **Out of scope.** The broader fact that `make:*`, `go:*`, `mise:*`,
   `powershell:*`, `pwsh:*` and `wsl:*` already run anything, including a
   branch's `tools/workflow/` (`make -C .scratch/<name> <target>` whose recipe
   calls it, or `go test -exec`), recorded in the archived designs and
   unchanged; a session on a host-affinity step that switches the main
   checkout to a feature branch runs that branch's `tools/workflow/` through
   `Bash(tools/workflow/*)` (an existing property, not widened here; named
   for a later pass); the `rm -rf /:*` deny the archived design flagged.

## Questions

### Blocking

None.

### Assumed

- **a1. How a branch's own script is run** (Decision 1). Proceeding with the
  main-owned launcher and no new allow rule. Options:
  - (a) `tools/workflow/native/native-run.sh -- .scratch/<name>/tools/<path>`,
    checked against tracked files of a real worktree, `tools/workflow/`
    refused;
  - (b) `Bash(.scratch/*/tools/*)` with a `Bash(.scratch/*/tools/workflow/*)`
    deny, accepted as an accident guard as wide as `.scratch/*`;
  - (c) `Bash(.scratch/*/tools/relay-vm/*)` only, widened per incident.

  Recommended: (a).
- **a2. How a `WARDEN_*` variable reaches a branch command** (Decision 2).
  Proceeding with the same launcher. Options:
  - (a) `native-run.sh WARDEN_X=value -- <command>`, `WARDEN_*` names minus
    secret-looking suffixes, the allowlist's own command shapes only;
  - (b) allow rules spelling the assignment, for example
    `Bash(WARDEN_RELAY_BOOT_VHDX=* .scratch/*/dist/warden*)`, tested before
    merge;
  - (c) no change: the scratch Makefile (or the `powershell` form),
    documented.

  Recommended: (a).
- **a3. Which commands the launcher accepts with variables** (Decision 2).
  Proceeding with the branch's and `main`'s `warden`, a branch's tracked
  `tools/` scripts outside `tools/workflow/`, and `go test` (in place or
  `-C .scratch/<name>`). Options:
  - (a) as stated;
  - (b) `warden` only;
  - (c) any command.

  Recommended: (a); `go test` covers the `WARDEN_HYPERV_*` live tests.
- **a4. Refused cases and workaround launchers** (Decision 3). Drafted as:
  the skill forbidding a Makefile or script written only to reach a refused
  command, and releasing the step with the refused command as the reason.
  **Answered in review r1: the scratch Makefile is allowed** (the draft's
  option (c)); Decision 3 is amended to match. Options as drafted:
  - (a) as stated;
  - (b) the skill says nothing;
  - (c) the skill documents the scratch Makefile as the fallback.

  Recommended: (a).
- **a5. Validation before merge** (Validation plan). Proceeding with tier 0
  plus a matcher check at x2-implement: any Claude Code session runs
  `claude -p --settings tools/workflow/native/settings.native.json` from the
  branch against the test's stub worktree and records which forms are
  allowed and refused in Consolidation; the first windows-desktop run after
  merge is the native check, recorded in its digest. Options:
  - (a) as stated;
  - (b) tier 0 only, the first native run after merge as the check;
  - (c) also an attended windows-desktop session on the branch before merge.

  Recommended: (a); the matcher is the same on every platform, and a rule
  whose text looked right but never matched is the bug the archived design
  fixed.
- **a6. The `host:*` hold** (Decision 5). Proceeding with keeping it.
  Options:
  - (a) keep `holds: [host:*]`;
  - (b) drop it: no native run can see the branch before merge;
  - (c) narrow it to the Hyper-V hosts.

  Recommended: (a); no host step is pending today, and it stops native runs
  from meeting the gap again before the skill text lands. Revisit with (b) if
  a blocking host step appears before x3-merge.

## Design

`tools/workflow/native/settings.native.json`: no change.

`tools/workflow/native/native-run.sh` (new, POSIX `sh`, mode 755): a header
comment with the usage and why each check exists (GLOBAL-01), then the checks
of Decisions 1 and 2, in this order: split at `--` (refuse if absent);
validate each assignment's name; match the command's first word(s) against
the shapes; for a `.scratch/<name>/tools/<path>` target, the component,
`workflow/`, worktree and `ls-files` checks; then `export` and `exec`. Run
from the checkout root; the shape checks are on the command's literal text
and the tracked-file check is on the worktree's index.

`.claude/skills/workflow-native/SKILL.md`, from "A branch under test" on
line 42 through line 48, becomes:

> A branch under test goes in a worktree under `.scratch/` (`git worktree add
> .scratch/<name> <ref>`), driven with `git -C .scratch/<name> ...`, with its
> binaries run as `.scratch/<name>/dist/warden ...` and its targets with
> `make -C .scratch/<name> <target>`, all from the checkout root. Its scripts
> run as `tools/workflow/native/native-run.sh -- .scratch/<name>/tools/<path>
> ...` (never its `tools/workflow/`), and a command that needs a `WARDEN_*`
> variable as `tools/workflow/native/native-run.sh WARDEN_X=value -- <command>`,
> never as `WARDEN_X=value <command>`. A command the procedure needs that is
> outside those forms and still refused goes in a scratch Makefile under
> `.scratch/<name>/`, run with `make -C .scratch/<name> <target>`; the digest
> names the target and the exact commands its recipe runs, and lists it under
> **Blocked on Jeff** as an allowlist gap. A case neither form can run is
> handled as **Record** says. A session that still cannot commit or push leaves its
> changes for the wrapper, which saves them and reports the run as failed.

**Record** (lines 84-99) gains, after the later-phase bullet:

> - A case the allowlist refused and no scratch Makefile could run: release the step with the refused command
>   as the reason and leave it pending (never `wf done` with the case
>   skipped); a backlog entry stays open with the same note.

No `docs/development/` rule changes (`rule_changes: []`): the forms are
procedure, which lives in the skill (FLOW-14). `overview.md` describes product
components, not the loop, so FLOW-15 asks nothing of it.

## Cross-driver impact

None: no driver code changes; the launcher and its test are loop tooling.

## Security note

`security_boundary: none`. No build-environment, relay, ledger or collector
code changes. The weighing:

- **What a malicious branch could already do.** A native session builds and
  runs a branch's code unattended: `.scratch/*/dist/warden*` is branch-built,
  `make -C .scratch/<name>` runs branch recipes, `go test` and `go run` run
  branch code, and `powershell`, `wsl` and `mise exec` run anything. The
  2026-11-04 run itself used `go run -C .scratch/dhcpprobe` and a scratch
  Makefile. A branch's tracked `tools/` script adds no capability beyond
  these.
- **What the launcher refuses, and what it cannot.** It refuses a file the
  session wrote, a path that leaves the worktree by `..`, a branch's
  `tools/workflow/`, and a secret-looking variable. These keep sessions on
  documented, reviewable forms; they are not a boundary: `make:*`, `go:*`
  (including `go test -exec`) and `powershell:*` still reach a branch's
  `tools/workflow/` or any command. The pushes the archived design denies
  stay denied whatever the `-C` directory; branch protection on `main` and
  FLOW-08 remain the real boundary. `curl`, `wget` and `sudo` stay denied as
  command text (and those denies match past an assignment).
- **Variables.** Only `WARDEN_*` names pass, so the launcher cannot set
  `PATH`, `LD_PRELOAD`, `GIT_*` or proxy variables. Some `WARDEN_*` names
  matter: `WARDEN_CTR_CLI` chooses the container CLI `warden` runs
  (`cmd/warden/config.go:97-98`), and `WARDEN_UPSTREAM_CA_CERTS` adds
  upstream CA certificates the relay trusts (`config.go:106-108`). Both are
  accepted: the binary that reads them is branch-built and could hardcode
  either, and `go:*` already runs anything. `WARDEN_QEMU_VNC_PASSWORD` and
  any other name ending in `_PASSWORD`, `_TOKEN`, `_SECRET` or `_KEY` is
  refused, because the launcher's command line is in the transcript
  (FLOW-18).
- **Not a sandbox.** The existing rules and the launcher filter command text,
  as the permissions reference says; they make the digest's commands the
  real ones.

## Validation plan

- **Tier 0, `tools/workflow/native/native_test.go`** (TEST-01, TEST-02), as
  Decision 4: `TestNativeSettingsRules` and `TestNativeRun`.
- `make lint` and `make test` (FLOW-17); `make enforcement` and
  `tools/workflow/flow18-check.sh` as for any process item.
- **Matcher check at x2-implement (a5):** with the branch's settings file,
  `claude -p --settings tools/workflow/native/settings.native.json` asked to
  run, against the stub worktree, `tools/workflow/native/native-run.sh --
  .scratch/t/tools/x.sh` and `tools/workflow/native/native-run.sh WARDEN_A=1
  -- .scratch/t/dist/warden` (expected allowed), and
  `WARDEN_A=1 .scratch/t/dist/warden` and `.scratch/t/tools/x.sh` (expected
  refused). The results go in Consolidation.
- **Check after merge:** the first windows-desktop run that validates a
  Hyper-V branch runs `refresh-cache.sh --check` and a
  `WARDEN_RELAY_BOOT_VHDX` case through the launcher without a denial or a
  scratch Makefile (the launcher covers both), and its digest says so. No native queue step (the session
  reads `main`'s settings), so no blocking native validation (FLOW-09).

## Implementation plan

Concrete at x2-implement (process item, FLOW-03); one commit each:

1. `tools/workflow/native/native-run.sh` (new, POSIX `sh`, mode 755;
   FLOW-14, FLOW-18, GLOBAL-01): Decisions 1 and 2 in the Design's order.
   Refuse unless run from a checkout's top level; split at `--`; check each
   assignment's name (`WARDEN_[A-Z0-9_]+`, no secret suffix); match the
   command against the shapes; for `.scratch/<name>/tools/<path>`, the
   component and `workflow/` checks, then `.scratch/<name>` is the top level
   of a worktree sharing this checkout's git common directory, and
   `ls-files --error-unmatch` on the target; then `export` and `exec`. Every
   refusal is one stderr line naming the reason and the accepted shapes,
   exit 2, before anything runs.
2. `tools/workflow/native/native_test.go` (new, test-only package
   `native`; TEST-01, TEST-02, GLOBAL-01, GLOBAL-02): Decision 4's
   `TestNativeSettingsRules` and table-driven `TestNativeRun`, skipped where
   `sh` or `git` is missing.
3. `.claude/skills/workflow-native/SKILL.md` (FLOW-14): the "A branch under
   test" paragraph and the **Record** bullet, as worded in Design.
4. `docs/book/src/contributing/structure.md` (UX-01 for contributor docs):
   the `native/` line names the launcher and its test.
5. `docs/book/src/concepts/overview.md` (FLOW-15) and
   `tools/workflow/native/settings.native.json`: checked, no change.
6. The a5 matcher check, recorded in Consolidation; `make lint`,
   `make test`, `make enforcement`, `tools/workflow/flow18-check.sh`,
   `dist/wf validate` and `dist/wf check -feature
   native-allowlist-scratch-tools` (FLOW-10, FLOW-17); Consolidation
   filled. Archive (FLOW-12) and the PR (FLOW-08) are Publish's.
7. At x4-close, on the `workflow` branch (FLOW-03): nothing beyond the queue
   bookkeeping; the 2026-11-04 Makefile evidence stays as recorded.

## Design review

- **r1, 2026-11-06: approved at `706564f`, read at `63c7b3d`** (Jeff,
  workflow-human, x1-design, FLOW-19). He reviewed it alongside
  regression-fixes-follow-up on one review page, with the assumptions first.
  - **a1, a2, a3, a5, a6: confirmed (a).** That covers the launcher for a
    branch's tracked `tools/` scripts and `WARDEN_*` variables, the
    validation plan, and keeping the `host:*` hold.
  - **a4: allow the Makefile pattern.** This is the draft's option (c). A
    command outside the launcher's forms runs from a scratch Makefile, and
    the digest names its target, its commands and the allowlist gap. It was
    folded into Decision 3, the Design text for the skill, and the
    Validation plan before the approval (`706564f`).

## Consolidation

Filled in at x2-implement (FLOW-10, FLOW-15).

## Rollback notes

Revert the PR. `native-run.sh`, its test and the skill's paragraph and
**Record** bullet go with the revert; a branch's `tools/` scripts and
`WARDEN_*` cases prompt as before, so an unattended run records them as
blocked. `settings.native.json` is unchanged either way, and no queue state
depends on the launcher.

## Archive summary

Filled in at Publish (FLOW-12).
