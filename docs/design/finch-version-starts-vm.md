---
feature: finch-version-starts-vm
status: draft
drivers: [container]     # driver/container only; no other driver tree is reached
seams: []                # the driver contract's wire and requirements are unchanged: whether
                         # a run is attended is read by the driver itself (Decision 3, a1),
                         # not passed by warden; option (B) of a1 would make it a seam change
surfaces: [output]       # finch's "not running" messages name `finch vm status` and the
                         # state; a build at a terminal prints the VM's start (UX-01)
repos: []                # this repository only
merge_first: []
areas: [driver/container, book, designs]
                         # driver/container: runtime.go, command.go and their tests; book:
                         # the guide's cli.md and configuration.md, overview.md's container
                         # pseudocode (FLOW-15); designs: this design
security_boundary: none  # no isolation, relay or ledger code is reached: the change is which
                         # host commands the driver runs to see and start finch's VM
alters_relationships: false
design_review: required  # blocking native validation (macos-finch) and assumed questions,
                         # flagged ones among them, rule out skip (FLOW-19)
native_validation:
  - {host: macos-finch, blocking: true, reason: "the defect and the fix are finch's own behaviour on macOS (its version query starts a stopped VM; vm status and vm start); no fake runtime in tier 0 shows what finch prints per state, that vm status leaves the VM stopped, or a real attended start"}
tier2_gaps: []
rules: [GLOBAL-09, GLOBAL-10, GLOBAL-11, UX-01, UX-04, UX-08, UX-09, ORCH-01, TEST-01,
        TEST-02, TEST-04, FLOW-01, FLOW-02, FLOW-06, FLOW-07, FLOW-09, FLOW-12, FLOW-15,
        FLOW-19]
rule_changes: []
concepts: [container-runtime, runtime-detection, runtime-check, finch, finch-vm, attended-run,
           doctor, probe]
related: [driver-contract.md, runtime-cli-project-path.md]
---

# The container runtime check starts a stopped finch VM and then reports it as failed

## Problem

driver-contract's Decision 26 (r4, CI repair, Jeff 2026-11-22) made the
container runtime check the runtime's server-version query: `docker version
--format '{{.Server.Version}}'`, the same for `podman`, and `finch version`
bare. Detection gives each candidate 3 s; a named runtime's first engine call
in a build, and `warden doctor`'s check of a named runtime and its re-ask of
each candidate, give it 10 s, and a candidate that answers only in the second
window gets doctor's slow hint. For finch the decision rests on a premise
written from finch's source: "finch's own `version` asks nerdctl in its VM and
fails when the VM is not running" (`versionQuery`'s comment in
`driver/container/runtime.go`).

On finch v1.19.0 that premise does not hold. The macbook's native run of
`driver-contract-runtime-check-finch` on `main` `c52926d`
(host:macos-finch/macbook.20261123T220740Z, log
`native/macos-finch/2026-11-23/runtime-check.log` on the `workflow` branch)
found:

- `finch version` with the VM stopped **starts the VM** and exits 0 once it is
  up, after 11.8 s (`finch info` does the same).
- `warden build --runtime finch` with the VM stopped kills the query at 10 s
  and fails with "container runtime finch is installed but not running
  (`finch version` failed)". The VM finishes starting anyway; the next build
  passes.
- `warden doctor --runtime finch` is blocked the same way after 10 s, and
  `warden doctor` with nothing named after 4.5 s (detection's 3 s, then the
  re-ask cut short); both leave the VM Running.
- No slow hint is seen: the VM's start takes longer than either window.

So the check has a side effect nobody chose (it starts the VM), its error is
wrong after the fact (the runtime it calls "not running" is starting), and its
result depends on how fast the VM boots.

**Jeff's decision** (2026-11-23, attended macbook session, recorded in the
queue item's `source`): "the check uses `finch vm status` as its confirmation
that the finch binary works and the VM is available, in place of `finch
version`. A VM that is not running is started only when the run is attended
(a person at a terminal), with the start shown; unattended, warden refuses up
front with the fixable error and the `finch vm start` hint, as today." This
design implements that decision; its assumed questions are the details the
sentence does not settle.

## Prior art

- **driver-contract** (archived), Decision 26, Behaviour 20, question a28 and
  the **Defaults and overrides** rows on the container runtime: the check,
  its windows (3 s detection, 10 s for a named runtime and doctor's re-ask),
  the slow hint, the "installed but not running" error with the start hint,
  the refusal to try detection in a named runtime's place, and "the probe
  makes no engine call". This design keeps all of them for docker and podman,
  and for finch replaces the version query with `finch vm status` and adds
  the attended start, as Jeff decided on 2026-11-23. It corrects Decision
  26's premise about `finch version`, which the native run disproved. Where
  finch's refusal now happens (the probe, Decision 4) departs from Decision
  26's placement for finch only, read from Jeff's "refuses up front" (a4,
  flagged).
- **runtime-cli-project-path** (archived): the supported runtimes and their
  values, unchanged. The detection order (finch, podman, docker) is
  driver-contract's Behaviour 21, unchanged.
- No other design in `docs/design/` has `finch`, `runtime-check` or
  `runtime-detection` among its concepts; driver-contract already has
  front-matter (FLOW-12). Nothing in the repository reads whether a run is
  attended today (`cmd/warden` and `driver/` have no terminal test), so
  Decision 3 is new.

## Decisions

1. **finch's check is `finch vm status`, in place of `finch version`** (Jeff,
   2026-11-23). On macOS and Windows the container driver asks finch `vm
   status` wherever it asked `finch version`: in detection (3 s), in a build's
   check of a named finch, and in doctor (10 s). Its trimmed stdout decides:
   `Running` is up; `Stopped` is down, with the start hint `finch vm start`;
   `Nonexistent` is down, with the hint `finch vm init`; a failure, a timeout
   or any other output is down too, and the error quotes what finch printed
   (Decision 5). The status query never starts the VM. docker and podman keep
   their version query. Alternatives:
   - (b) **ask `vm status` first and then `finch version` for a Running VM**,
     so the engine inside is still asked: Jeff's sentence puts the status
     query in place of the version query, and a Running VM whose nerdctl does
     not answer fails at the build's first container call with the runtime's
     own error, which is clear enough. Rejected.
   - (c) **lengthen finch's windows** so its version query can finish
     starting the VM: departs from the 3 s and 10 s and starts the VM
     unattended. Rejected.
2. **On Linux, finch keeps its version query.** Linux finch runs without a VM
   and has no `vm` command, so there the check is `finch version` as on
   `main`, and no VM is ever started. The choice is by the host's platform
   (`runtime.GOOS`), fixed at build time, not a fallback taken when `vm
   status` fails (a2). Alternative: try `vm status` everywhere and fall back
   to `finch version` when it fails or prints an unknown state; a fallback
   that, on macOS, runs the very query that starts the VM. Rejected.
3. **A run is attended when the driver has a controlling terminal**
   (a1, flagged). `warden` runs the driver with pipes for its standard
   streams (DRV-15, DRV-16; `cmd/warden/drivercall.go`), so the driver cannot
   ask whether they are terminals; it can ask whether its process has a
   controlling terminal, which it inherits from `warden`: on macOS and Linux
   `/dev/tty` opens, on Windows `CONIN$` opens. A shell at a terminal has one
   whatever its redirections; launchd, a scheduled session, CI and a process
   started with `setsid` have none. Alternatives:
   - (b) **`warden` decides** (its standard input and standard error are both
     terminals) and passes it to the driver in `run`'s request: the truer
     test of "a person at a terminal", but a new request field, so a change
     to the driver-contract seam (`specs/driver-contract/v1.md`) for one
     driver's need. Not chosen; a1 names it.
   - (c) **an explicit setting** (`[driver.container] start_vm`): not what
     Jeff decided. Rejected.
4. **A stopped finch is refused in the probe of `run` when unattended, and
   started by `run` when attended** (Jeff, 2026-11-23; a4, flagged). The
   probe asks `finch vm status`. `Running` passes. `Stopped`, unattended, is
   refused there with the fixable error and the `finch vm start` hint,
   before any output directory exists ("up front"). `Stopped`, attended,
   passes the probe; `run` then, before it writes the state file's runtime or
   creates anything, prints one line, "starting finch's VM (finch vm start);
   the build waits for it", runs `finch vm start` with its output on the
   driver's standard error, which `warden` shows, and asks `vm status` again:
   `Running` goes on with the build; anything else fails it with the fixable
   error and the start hint. `Nonexistent` or an unknown state is never
   started: `finch vm init` creates a VM, which is setup, not a start. The VM
   is left running after the build, as `finch vm start` by hand leaves it.
   This replaces, for finch on macOS and Windows, Decision 26's "the probe
   makes no engine call" and the late failure at the build's first engine
   call; `vm status` asks the VM manager (lima), not the engine.
5. **The messages name `finch vm status` and the state.** Unattended or in
   doctor: "container runtime finch is installed but its VM is not running
   (`finch vm status`: Stopped)", fix "start its VM: finch vm start; a build
   run at a terminal starts it". `Nonexistent`: "... has no VM (`finch vm
   status`: Nonexistent)", fix "create it: finch vm init". A failed or
   unknown status: "... `finch vm status` failed: <its first line, quoted>",
   fix "check finch: finch vm status". A named finch says where it was named,
   as today (`namedRuntimeFix`).
6. **Doctor never starts the VM** (a3). It is the health check (UX-04): it
   asks `finch vm status` (named: 10 s; detected: 3 s, as the build does) and
   is blocked with Decision 5's error when finch is the runtime it would
   report and the VM is not Running. Its re-ask with 10 s after detection
   finds none up asks finch `vm status` again, once; a `Stopped` finch stays
   "not running", not the slow hint.
7. **Detection with nothing named prefers a runtime that is up over starting
   finch** (a5). Detection asks finch, podman, docker in order; a stopped
   finch is passed over like any runtime that is down. Only when none is up,
   finch is installed and its VM is `Stopped`, and the run is attended, does
   `run` start finch's VM and use finch; doctor, `clean`, `reap` and `attach`
   detect without starting, as today.

## Questions

### Blocking

None. Jeff's decision of 2026-11-23 settles the check and when a VM is
started; the questions below are its details.

### Assumed

- **a1. "Attended" is the driver having a controlling terminal (Decision 3).**
  Proceeding with (A): the container driver reads a run as attended when
  `/dev/tty` (macOS, Linux) or `CONIN$` (Windows) opens; no request field, no
  setting. Options: (A) as stated, no seam change; (B) `warden` tests its own
  standard input and standard error with isatty and passes `attended` in
  `run`'s request, a change to `specs/driver-contract/v1.md` (an optional
  field, DRV-14's request); (C) an explicit setting, `[driver.container]
  start_vm`. Recommended: (A). Flagged: it defines Jeff's "attended" for the
  first time, and (B) is a change to the driver-contract seam.
- **a2. Linux finch keeps `finch version` (Decision 2).** Proceeding with (A):
  by `runtime.GOOS`, Linux finch is checked with its version query as on
  `main` and never started. Options: (A) as stated; (B) `vm status` on every
  host, falling back to `finch version` when it fails. Recommended: (A), no
  fallback.
- **a3. Doctor never starts the VM (Decision 6).** Proceeding with (A):
  doctor reports a stopped finch as blocked with the start hint, attended or
  not. Options: (A) as stated; (B) doctor at a terminal starts it as a build
  does. Recommended: (A): doctor is a check (UX-04), and Jeff's "the run" is
  read as a build or shell.
- **a4. The unattended refusal is in the probe of `run`, before any output
  directory (Decision 4).** Proceeding with (A): for finch on macOS and
  Windows the probe asks `vm status` and refuses a stopped VM there; the
  start, when attended, is in `run`, outside the probe's 30 s limit. Options:
  (A) as stated; (B) keep Decision 26's placement: the probe asks nothing,
  and `run`'s first engine call refuses or starts, leaving the output
  directory. Recommended: (A), from Jeff's "refuses up front". Flagged: it
  departs from Decision 26 (Jeff, 2026-11-22: "the probe makes no engine
  call"; an engine that is down fails at the build's first call) for finch,
  on a reading of his newer words.
- **a5. Detection passes a stopped finch over and starts it only when nothing
  else is up (Decision 7).** Proceeding with (A). Options: (A) as stated; (B)
  finch first means finch: an attended build starts finch's VM even when
  podman or docker is up; (C) detection never starts a VM; only a named
  finch is started. Recommended: (A): a runtime that is up is not replaced by
  one that must be started, and a user with only finch gets the start.
- **a6. The start has no time limit of its own (Decision 4).** Proceeding
  with (A): `finch vm start` runs until it ends; the person at the terminal
  sees its output and can interrupt it, and `warden`'s signal handling stops
  the driver as for any build. Options: (A) as stated; (B) a limit, such as
  120 s, a new default with no override. Recommended: (A).

## Design

All of it is in `driver/container`.

- **`runtime.go`.** A new `finchVMState(timeout)` runs `finch vm status`
  under a context with `WaitDelay` 1 s, as the version query does, and
  returns `Running`, `Stopped`, `Nonexistent` or "unknown" with the first
  line of what finch printed. `runtimeAnswers(name, timeout)`, for finch on
  macOS and Windows, is `finchVMState(timeout) == Running`; elsewhere it is
  as today. `versionQuery` and `versionCommand` stay for docker, podman and
  Linux finch, and their comment drops the disproved premise and says what
  finch v1.19.0 does. `detect` keeps its order and returns, beside the
  runtime that answered and the installed list, whether finch is installed
  with its VM `Stopped`. The messages are Decision 5's: `notAnswering`,
  `checkEngine`'s and `doctorRuntime`'s named branch name `finch vm status`
  and the state for finch. `checkEngine` asks nothing of finch on macOS and
  Windows: the probe has.
- **`attended.go`** (new, with `_unix.go` and `_windows.go` halves):
  `attended()` opens `/dev/tty` or `CONIN$` and closes it, true when the open
  succeeds. Tests replace it, as they replace `lookPath`.
- **`command.go`.** `preflight` (`checkedRunner`): for finch on macOS and
  Windows, named or detected, it asks the VM's state; `Running` passes;
  `Stopped` passes when `attended()`, and is refused with Decision 5's error
  otherwise; `Nonexistent` or unknown is refused. With nothing named and
  detection finding none up, a stopped finch, attended, is the runtime
  (Decision 7). `run`: after `buildRunner`, if the runtime is finch and its
  state is `Stopped`, it starts the VM (Decision 4) before the state file is
  written; the line and finch's output go to the driver's standard error.
  `doctor`, `reap`, `clean` and `attach` never start it.
- **Docs (UX-01, FLOW-15).** `docs/book/src/guide/cli.md`: doctor's health
  check names `finch vm status` for finch, and says doctor never starts the
  VM. `docs/book/src/guide/configuration.md`: the example error for a
  stopped finch, and a paragraph on the attended start ("at a terminal, a
  build starts a stopped finch VM and says so; elsewhere it is refused before
  anything is created"). `docs/book/src/concepts/overview.md`: the container
  driver's pseudocode (detection, probe, build and doctor lines).

The area map has what this needs (`driver/container`, `book`, `designs`);
nothing is missing from it. There is no other repository.

## Defaults and overrides

- **New default: an attended build starts a stopped finch VM** (Decision 4,
  Jeff 2026-11-23). Why obvious: the person asked for a build on finch and
  is there to see the start; unattended, it is refused, as today. Override:
  none of its own (a1 option C, a setting, is not chosen); naming another
  runtime (`--runtime`, `WARDEN_CTR_CLI`, `[driver.container] cli`) avoids
  finch, and starting the VM beforehand makes the start unneeded. How the
  user sees it: the line "starting finch's VM (finch vm start); the build
  waits for it" and finch's own output.
- **Changed: finch's check on macOS and Windows** is `finch vm status` in
  the same windows (3 s, 10 s), in place of `finch version`. The windows and
  the slow hint are unchanged for docker and podman. No override changes:
  `--runtime`, `WARDEN_CTR_CLI` and `[driver.container] cli` are used
  exclusively, and a named runtime is never replaced by detection.
- **Changed: where a stopped finch is refused**: the probe of `run`, before
  any output directory (a4), where Decision 26 refused at the build's first
  engine call and left the output directory.
- **No time limit on the start** (a6).
- **Fallbacks: none.** Linux finch keeps its version query by platform, not
  as a fallback (Decision 2, a2). A `vm status` that fails or prints an
  unknown state is an error, never followed by `finch version` (which would
  start the VM). The fallback dropped is a2's option (B).

## Cross-driver impact

- **container**: finch's runtime check and the attended start, as above.
  docker and podman are unchanged.
- **qemu, vz, hyperv, firecracker**: unaffected; none runs a container
  runtime, and nothing in `driver/` outside `driver/container` is reached.

## Validation plan

- **Tier 0** (`make test`, every platform; TEST-02, TEST-04): the fake finch
  in `driver/container`'s tests answers `vm status` from its fixture
  (Running, Stopped, Nonexistent, an unknown word, a failure, a sleep past
  the deadline) and `vm start` (changes the fixture to Running, or fails),
  and records each call; `attended` is replaced by the test. Tests: finch's
  `version` is never run on macOS and Windows; each state's message and fix
  (Decision 5); a stopped finch, unattended, is refused by the probe and
  `run` never ran `vm start`; attended, the probe passes, `run` prints the
  line, runs `vm start` once, asks the state again and goes on, or fails
  with the start hint when the state is not Running after it; a Nonexistent
  finch is never started; detection with podman up and finch stopped
  chooses podman and starts nothing (a5); with none up and finch stopped,
  attended `run` starts finch and doctor is blocked without starting it
  (a3); `TestDoctorAsksEachRuntimeAtMostTwice` and the slow-hint tests still
  pass; on Linux the existing finch version-query tests still pass (a2).
- **Tier 1**: unchanged checks.
- **Tier 2, native**: **macos-finch, blocking** (FLOW-09). Cases, in the
  queue step `f6-validate-macos-finch`:
  1. VM Running: `finch vm status` prints `Running` (record exit code and
     time); `warden doctor --runtime finch` and `warden doctor` print
     `container runtime: finch` and `ok`; a default build succeeds and its
     ledger verifies.
  2. `finch vm stop`; `finch vm status` prints `Stopped` (exit code, time);
     run three times, the VM is still `Stopped`.
  3. Unattended (the session's own shell, no controlling terminal; record
     that `/dev/tty` does not open): with the VM stopped, `time warden build
     --runtime finch` is refused within 2 s with Decision 5's error and the
     `finch vm start` hint, leaves no output directory, and the VM is still
     `Stopped`; the same with nothing named (`warden build`).
  4. With the VM stopped, `time warden doctor --runtime finch` and `time
     warden doctor` are each blocked within 2 s with the state, exit 1 with
     no `ok`, and leave the VM `Stopped`; the same under `script -q /dev/null
     ...` (a controlling terminal): doctor does not start it.
  5. Attended, simulated with `script -q /dev/null dist/warden build
     --runtime finch ...` (or by Jeff at a terminal if the session cannot run
     `script`): with the VM stopped, the build prints the start line and
     finch's output, the VM is `Running`, and the build succeeds and
     verifies. The same with nothing named.
  6. `go test ./driver/container/...` passes on macOS.
  Evidence under `native/macos-finch/<date>/`. `finch vm remove` and `finch
  vm init` are not run (`Nonexistent` is tier 0 only). The VM is left
  Running, as found.
- **Not covered natively, no gap filed**: Windows finch (WSL2, `CONIN$`) and
  Linux finch. No native host has either; Linux finch's path is today's
  version query, and both are covered by tier 0 on their platforms in CI.

## Implementation plan

1. `driver/container/attended*.go`: `attended()` per platform, replaceable
   in tests (Decision 3; TEST-04).
2. `driver/container/runtime.go`: `finchVMState`, finch's check on macOS and
   Windows, `detect`'s stopped-finch result, the messages, `versionQuery`'s
   comment corrected (Decisions 1, 2, 5, 6, 7; GLOBAL-10, UX-08, UX-09).
3. `driver/container/command.go`: the probe's refusal and `run`'s attended
   start before the state file (Decision 4; GLOBAL-11, UX-04).
4. Tests in `driver/container` as under **Validation plan** (TEST-01,
   TEST-02, TEST-04); the existing finch expectations in
   `runtime_answers_test.go` and `runtime_test.go` updated (rework of
   Decision 26's finch query, FLOW-06).
5. Docs: `cli.md`, `configuration.md`, `overview.md` (UX-01, FLOW-15).
6. `make test`, and `make lint` for linux, darwin and windows, since the new
   file has platform halves (FLOW-17).

## Design review

None yet.

## Consolidation

Filled in at Publish.

## Rollback notes

None.

## Archive summary

Filled in at Publish.
