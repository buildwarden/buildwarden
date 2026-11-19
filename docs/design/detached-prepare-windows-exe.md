---
feature: detached-prepare-windows-exe
status: draft
drivers: [container, qemu, vz, hyperv, firecracker]  # conservatively (FLOW-02, the TEST-02
                         # caveat): driver/detach.go is shared (rules.yaml drivers.shared), so
                         # every driver is declared, as project-config-trust did for the same file.
                         # Two drivers call driver.StartDetached and change (the hyperv base
                         # refresh, driver/hyperv/freshness.go, and the qemu Windows layer refresh,
                         # driver/qemu/driver.go); container, vz and firecracker do not call it and
                         # their trees are not edited: asserted unaffected (ORCH-01, Cross-driver
                         # impact; r2, question a5)
seams: []                # no claim, collector, executor or ledger change; the child's arguments are
                         # project-config-trust's, unchanged
surfaces: [output]       # one new stderr line when a due refresh cannot start; the existing
                         # "refreshing it in the background" line is unchanged
repos: []                # only this repository
areas: [driver-core, driver/hyperv, driver/qemu, book, designs]
                         # driver/detach.go (+ test); the two callers; drivers.md and the overview
                         # line (UX-01, FLOW-15); this design
security_boundary: host-env  # which executable warden runs on the host, before any isolation
                         # exists; it prepares the base every later build environment boots from
                         # (Security note)
alters_relationships: false  # inside the orchestrator's and the two drivers' existing roles; no
                         # seam, bootstrap, owner or driver.Driver / driver.Session change
design_review: required  # FLOW-19: a security boundary, blocking native validation, assumed
                         # questions, and areas declared where intake recorded none. r2: drivers,
                         # an impact field, changed since approved at b7619b5, and a5 is open, so
                         # the delta needs a re-review
native_validation:
  - {host: windows-hyperv, blocking: true, reason: "the fault is Windows-only (os/exec's PATHEXT lookup) and the change reaches driver/hyperv; CI runs no driver tests on Windows, so only this host shows the extension-less dist/warden starting itself, a warden.exe beside it not being run, and the hyperv refresh line and log"}
  - {host: macos-qemu, blocking: false, reason: "on macOS the start is the same exec(2) of the same absolute path, which Linux CI exercises in the new test; what only this host adds is a qemu Windows layer refresh end to end, which does not complete on main today (qemu-windows-guest-package)"}
tier2_gaps:
  - {host: macos-qemu, gap: "a qemu Windows guest build whose prepared layer is past [vm] image_refresh_after prints the 'refreshing it in the background' line and its detached prepare runs from the warden that was started; with the start made to fail, the build prints the new warning line and still builds (backlog detached-prepare-windows-exe-qemu)"}
rules: [GLOBAL-01, GLOBAL-06, GLOBAL-07, GLOBAL-09, GLOBAL-10, GLOBAL-11, ORCH-01, ORCH-05,
        UX-01, UX-04, TEST-01, TEST-02, TEST-03, TEST-04, TEST-08, SEC-01, FLOW-01, FLOW-02, FLOW-05,
        FLOW-07, FLOW-09, FLOW-12, FLOW-14, FLOW-15, FLOW-19, FLOW-20, LOOP-02, LOOP-04]
rule_changes: []
concepts: [image-refresh, detached-prepare, self-exec, os-executable, windows-pathext,
           executable-extension, dev-build, silent-failure, security-fix]
related: [project-config-trust.md, hyperv-windows-prepare.md]
---

# The background image prepare runs the file warden was started from

## Problem

A Hyper-V build whose prepared base is `[vm] image_refresh_after` old, and a
qemu Windows guest build whose prepared layer is, start
`warden image prepare` in the background through `driver.StartDetached`
(`driver/detach.go`). It runs `exec.Command(os.Executable(), ...)`.

On Windows, `os/exec` resolves the extension of every path it is given,
absolute ones included: `exec.Command` and again `Cmd.Start` call
`lookExtensions`, which for a name with no extension tries only the name
plus each `PATHEXT` ending (`.COM`, `.EXE`, ...) and never the name itself
(Go 1.26.3, `os/exec/exec.go` and `lp_windows.go`). `make build` writes
`dist/warden` on every OS, so on Windows `os.Executable()` is
`C:\...\dist\warden` and `os/exec` looks for `dist\warden.exe`:

- **No `warden.exe` beside it.** `Start` fails. Both callers drop the
  error (`if err := driver.StartDetached(...); err == nil { print }`), so
  the build prints nothing, `prepare.log` is left truncated to 0 bytes, no
  prepare runs, and the prepared base ages until builds are refused at
  `image_min_grace`.
- **A `warden.exe` beside it.** That other file is started. On the Windows
  desktop a stale `dist/warden.exe` of 2026-11-04 prepared the base for
  `main`'s build of 2026-11-16: the base every later build boots was made
  by code nobody chose to run.

Found by `host:windows-hyperv/20261116T051002Z` (unattended) while
validating `project-config-trust`'s gap `project-config-trust-hyperv`;
`main` 623131a has it, so it was filed as that item's follow-up (FLOW-09).
Evidence: `native/windows-hyperv/2026-11-15/` on the `workflow` branch
(`main-623131a-hyperv-refresh-due.log`,
`main-623131a-hyperv-refresh-due-no-exe-beside.log`,
`branch-3733c4b-hyperv-build.log`, `branch-3733c4b-hyperv-refresh-due-exe.log`).
A released `warden.exe` is not affected: its name has the extension.

**Under the freeze (FLOW-20).** This item is not a program item: its
`related` does not list `componentization`. It is exempt from the freeze as
a fix for a regression on `main` (FLOW-09), filed as the follow-up of
`project-config-trust`, itself a security fix. It also closes a host-side
"wrong executable runs" fault (Security note). FLOW-20 leaves the judgment
to intake and the design review: assumed question a4.

## Prior art

- **`project-config-trust`** (archived, `security_boundary: host-env`)
  made the detached prepare read the config file its build read
  (`detachedArgs`, its Decision 10). This item keeps `detachedArgs` and
  its test unchanged and replaces only how the process is started. Its
  Archive summary files this item. It edited the same shared file,
  `driver/detach.go`, and declared every driver for it, with no blocking
  native validation for the drivers whose code it did not change: the
  precedent a5 follows.
- **`hyperv-windows-prepare`** (archived) designed the refresh: by age, in
  the background, the build going on with the current base; only one
  prepare at a time under the `hyperv-prepare` lock; `doctor` warns once the
  base is past `image_refresh_after` with no refresh running. This item
  keeps every one of those; it only makes the start happen and makes a
  failed start visible.
- **GLOBAL-10 / GLOBAL-11.** The `PATHEXT` lookup is a second executable
  run in place of the one provided (`os.Executable()`), and only when the
  provided one cannot be resolved: an unnamed fallback, which both rules
  forbid. Dropping a failed start silently is what GLOBAL-11's "stops and
  says what failed" forbids.
- **ORCH-05** (Windows host is a 1.0 feature) and **GLOBAL-07**: the fix
  is one code path for every OS, with no build-tagged start.
- No other site re-executes warden through `os/exec`: `grep` for
  `os.Executable()` finds lookups of files beside the executable
  (`hostutil.go`, `relayvm/assets.go`, `vz`, `container`) and the Hyper-V
  service's registered path (`hyperv/service_windows.go`, which the SCM
  starts as given); none of them goes through `PATHEXT`.

## Decisions

1. **Start the file `os.Executable()` names, with no lookup.**
   `StartDetached` calls `os.StartProcess(exe, argv, attr)` with
   `argv[0] = exe`, the log as stdout and stderr, the null device as stdin,
   the inherited environment and `detachedAttr()`, then releases the
   process. `os.StartProcess` does no extension resolution: on Windows it
   passes `exe` to `CreateProcess` as the application name, which runs the
   named file whatever its extension; on Unix it is the same `fork`/`exec`
   of the same absolute path as today.
   - *Rejected: keep `exec.Command` and set `Cmd.Path`.* `Cmd.Start`
     re-runs `lookExtensions` on Windows for a `Cmd` not made by
     `Command` with that path, so the lookup cannot be bypassed in
     `os/exec`.
   - *Rejected: `exec.Command(exe + ".exe")` or resolving with
     `exec.LookPath` first.* Both run some file other than the one named
     whenever the names differ.
   - *Rejected: copy or hard-link the executable to `<exe>.exe`.* Writes
     beside the executable and leaves a file that could itself go stale:
     the same fault with more steps.
2. **A refresh that is due and does not start is reported, and the build
   goes on.** Each caller prints one line to its progress stream when
   `StartDetached` fails: what failed, the error, and the command that
   refreshes by hand (assumed question a2). The build continues on the
   current base, as it does when the refresh starts; nothing else runs in
   its place. The start error is also written to the log file, so a
   `prepare.log` of 0 bytes no longer means "nothing happened, no reason
   given". The qemu caller's `TryHostLock` error, dropped today the same
   way, is reported with the same line.
   - *Rejected: fail the build.* The base is still within grace (the
     `image_min_grace` refusal is the stop, and it already says how to
     fix it); stopping a build that can run correctly because a
     background chore failed serves nobody.
   - *Rejected: stay silent (today).* GLOBAL-11.
3. **`make build` keeps writing `dist/warden` on every OS** (assumed
   question a1). With Decision 1 the extension no longer matters to warden.
   The native sessions' permission allowlist matches `dist/warden` and not
   `dist/warden.exe` (digest 2026-11-02), and that allowlist is in
   `.claude/`, which only a process item changes (FLOW-14); CI and the
   docs name `dist/warden` too.
   - *Rejected: `dist/warden.exe` on Windows.* Breaks the native
     allowlist until a process item follows, and fixes nothing Decision 1
     leaves.
   - *Rejected: write both.* Two files that can drift: the stale-file
     fault this item removes.
4. **No new CI job.** CI runs no `driver` tests on Windows today; the new
   test runs there in the blocking `windows-hyperv` step. A Windows
   `go test ./driver/...` job is a wider change for its own item.

## Questions

### Blocking

None.

### Assumed

- **a1. `make build` on Windows.** Proceeding with (a); confirm.
  (a) keep `dist/warden` on every OS (Decision 3); (b) write
  `dist/warden.exe` when the target OS is Windows, and file a process item
  for the native allowlist; (c) write both. Recommended: (a).
- **a2. A due refresh that cannot start.** Proceeding with (a); confirm.
  (a) print one line naming the failure and
  `warden image prepare --driver hyperv` (qemu: the `--os windows --arch
  <arch>` form), write the error to the log, and go on with the build;
  (b) fail the build with that fix; (c) silent, as today. Recommended: (a).
- **a3. The stale `dist/warden.exe` on the Windows desktop.** Proceeding
  with (a); confirm. (a) the blocking `windows-hyperv` step uses it as the
  "file beside" in its case 3, shows it is not run, then deletes it
  (`dist/` is ignored by git); (b) leave it; (c) delete it before
  validating, and plant a fresh one for case 3. Recommended: (a).
- **a4. The freeze.** Proceeding with (a); confirm. (a) exempt from
  FLOW-20 as a fix for a regression on `main`, filed as the follow-up of
  a security fix; (b) put it on hold until the program ends.
  Recommended: (a).
- **a5 (r2). The `drivers` claim for a change to `driver/detach.go`.**
  *Flagged: it widens the declaration approved at `b7619b5`, `[qemu,
  hyperv]`; the behaviour and the "others unaffected" assertion are
  unchanged.* Proceeding with (a); confirm. (a) declare every driver,
  `[container, qemu, vz, hyperv, firecracker]`, as FLOW-02 asks of a change
  to a shared package and tier 1's `drivers-claim` enforces (ORCH-01), and
  keep "container, vz and firecracker unaffected" as ORCH-01's assertion
  (Cross-driver impact), with no native validation added for them; (b)
  declare `[interface]`; (c) keep `[qemu, hyperv]` and carry a
  `Tier1-Allow: ORCH-01` trailer on the commits that edit
  `driver/detach.go` (LINT-05). Recommended: (a). It is the precedent
  (`project-config-trust` declared all five for the same file; `http-body-
  path-drift` all five for a shared `relay/` change); (b) is for a change
  at the level of `driver.Driver` or `driver.Session` (FLOW-02), which this
  is not, and would overstate the change; (c) uses a suppression for a
  case the rule already answers, and `pre-release-cleanup` records Jeff
  rejecting `Tier1-Allow` trailers where a declaration or edit can satisfy
  the check.

## Design

`driver/detach.go`:

```
StartDetached(logPath, args...):
    exe = os.Executable()                     -- the file this process was started from
    open logPath (create, truncate)
    startDetached(exe, log, detachedArgs(args))
    on error: write "[warden] could not start <exe>: <err>" to the log (when it opened);
              return the error

startDetached(exe, log, args):                -- seam for the test
    null = open os.DevNull
    p = os.StartProcess(exe, [exe] + args,
          {Files: [null, log, log], Env: os.Environ(), Sys: detachedAttr()})
    p.Release()
```

`detachedArgs`, `detachedAttr` (`detach_windows.go`, `detach_unix.go`) and
`ConfigFile` are unchanged.

`driver/hyperv/freshness.go` (`ensureFreshBase`) and `driver/qemu/driver.go`
(`ensureFreshWindowsLayer`):

```
if refresh is due and no prepare holds the lock:
    if StartDetached(...) ok:
        "[warden] prepared base is <age> old; refreshing it in the background (log: <path>)"   -- unchanged
    else:
        "[warden] prepared base is <age> old, and its background refresh did not start: <err>;
         run `warden image prepare --driver hyperv` to refresh it"
    go on with the build on the current base
```

The qemu wording says "prepared Windows layer" and names
`warden image prepare --driver qemu --os windows --arch <arch>`, as its
existing line and `prepareFix(ga)` do. The qemu `TryHostLock` error gets
the same line with that error.

Docs: `docs/book/src/guide/drivers.md`, the Hyper-V paragraph on the
background re-prepare (and by reference the qemu Windows one): a refresh
that cannot start is reported with the command to run by hand, and the
build goes on. `overview.md`: the `"hyperv-prepare"` lock line gains
"a refresh that does not start is reported, the build goes on" (FLOW-15).

The area map has what this needs: `driver-core` (`driver/`),
`driver/hyperv`, `driver/qemu`, `book`, `designs`. No link is missing.

## Defaults and overrides

- **Which executable the background prepare runs** (default, a choice the
  code makes): the file `os.Executable()` names, exactly. It is what a user
  expects: the warden they ran is the warden that prepares. Nothing
  overrides it; there is no flag or variable for it, before or after.
- **The `PATHEXT` lookup, dropped.** Today, on Windows only, a second
  executable (`<exe>.exe` and the other `PATHEXT` endings) is run in place
  of the named one, and the named one is never tried. It was never named
  as a fallback; GLOBAL-10 and GLOBAL-11 forbid it. Removed by Decision 1;
  its alternatives are rejected there.
- **A due refresh that does not start** (a2): reported on the build's
  progress stream and in the log, with the command that refreshes by hand.
  This is not a fallback: the build goes on with the current base in both
  outcomes, and no other path or source runs because the start failed.
- **`make build`'s output name** (a1): unchanged, `dist/warden`.

No other default, override, merge or fallback is added or changed.

## Cross-driver impact

The claim is every driver (r2, a5): `driver/detach.go` is shared, and
FLOW-02 and tier 1's `drivers-claim` count a change to it as reaching every
driver (ORCH-01, the TEST-02 caveat). Two drivers change; the others are
asserted unaffected:

- **hyperv:** `ensureFreshBase` reports a failed start. Its refresh now
  starts from a `dist/warden` with no extension.
- **qemu:** `ensureFreshWindowsLayer` reports a failed start and a lock
  error. On macOS and Linux the start is the same `exec` of the same path.
- **container, vz, firecracker:** unaffected. None calls `StartDetached`
  (its only callers are the two above), their trees are not edited, and
  nothing else in `driver/` changes: `detachedArgs`, `detachedAttr`
  (`detach_unix.go`, `detach_windows.go`) and `ConfigFile` are untouched,
  and `StartDetached`'s signature is the same. `vz` refreshes its image
  only through `sudo warden image prepare`, by hand. Their code paths reach
  the changed function nowhere, so no native host of theirs can observe
  the change: no native validation is declared for them, blocking or
  tracked (TEST-02's caveat is met by the declaration and by tier 0 and the
  `integration` workflow, which runs on a change to the `driver/` root and
  builds the container driver with the new `driver/`).

## Security note

**Boundary: the host OS and the build environment** (GLOBAL-06,
`host-env`). The background prepare is warden acting on the host before any
build isolation exists, and what it produces, the prepared base or layer,
is the image every later build environment boots and the ledger records
as the environment's base.

**Posture before.** On Windows, with an extension-less executable, the
host ran whichever `<exe>.<PATHEXT ending>` existed beside warden in place
of warden itself: an older build, or any file of that name. The prepared
base was then made by code the user did not run, and the ledger described
a base whose preparing code was not the build's warden. With no such file
the refresh silently did not happen. In SEC-01's terms this gives an
attacker no new capability (one who can write `warden.exe` beside
`dist/warden` can replace `dist/warden`), but it breaks the guarantee that
the code preparing the build environment is the code the user chose.

**Posture after.** Exactly the file the running warden was started from is
run; a failure to start it is reported. No path, name or search is
involved, on any OS.

**TEST-08.** The security suite should exercise: on Windows, a
`warden.exe` planted beside an extension-less `dist/warden` is not run by a
refresh-due build (the started process's path is `dist\warden`), and the
prepare log is the started warden's. The blocking native step's case 3 is
that case; the tier-0 test below is its unit form.

## Validation plan

**Tier 0** (`make test`, every OS CI runs `go test ./...` on: Linux):

- `startDetached` with a copy of the test binary saved as `<dir>/warden`
  (no extension) and a second copy at `<dir>/warden.exe`: the child, in a
  helper mode the test selects by environment, writes its own
  `os.Executable()` to the log; the log names `<dir>/warden`. On Linux the
  `warden.exe` copy is inert, so the case is meaningful on Windows (native
  step case 1) and a guard against regressions elsewhere.
- A start that fails (a path that does not exist): `StartDetached` returns
  the error and the log holds the "could not start" line.
- The callers' line: each refresh function's output with a start that
  fails (the start function injected), the build going on.
- `TestDetachedArgs`, unchanged.

**Tier 1:** `make lint`, and `GOOS=windows make lint` (the build-tagged
`detach_windows.go` is unchanged, but `driver/hyperv` builds only there).

**Tier 2, blocking: `windows-hyperv`** (`f6-validate-windows-hyperv`), on
the branch, built with `make build` (so `dist/warden`, no extension):

1. `go test ./driver/ ./driver/hyperv/` on the Windows host: the tier-0
   cases above pass there.
2. No `warden.exe` in `dist/`: a hyperv build with
   `[vm] image_refresh_after = "1m"` prints "refreshing it in the
   background", `prepare.log` fills with the started warden's output (its
   config line first), the prepare finishes and the new base is active; a
   build meanwhile starts no second prepare.
3. A `warden.exe` beside `dist/warden` (the stale file of 2026-11-04,
   a3): the same build; the background process's path (`state.ps1`) is
   `dist\warden`, not `warden.exe`; then delete the stale file.
4. A refresh that does not start: with `prepare.log` held open with no
   sharing by another process (a PowerShell `[IO.File]::Open(..., 'None')`
   in a `state.ps1` variant), so the log cannot be opened, the refresh-due
   build prints the new line naming the error and
   `warden image prepare --driver hyperv`, and still SUCCEEDS. (A failure
   of the process start itself is the tier-0 case, run in case 1.)
5. `go build -o dist/warden.exe ./cmd/warden` (the released name): the
   refresh-due build still prints the line and the prepare runs from
   `dist\warden.exe`.

**Tier 2, tracked: `macos-qemu`** (backlog
`detached-prepare-windows-exe-qemu`): the `tier2_gaps` entry above. Not
blocking: the start on macOS is the `exec` of the same absolute path as
today, covered by tier 0 on Linux, and qemu Windows builds do not complete
on `main` (`qemu-windows-guest-package`).

## Implementation plan

Plan, 2026-11-18. Written from the design as its review approved it (r1,
**approved at `b7619b5`**; a1 to a4 confirmed as drafted, no
`rule_changes`), on the branch at `eeed552`, which is `origin/main`
`0cd3c02` plus this design (`main` has not moved since). Read for it:
`driver/detach.go`, `detach_unix.go`, `detach_windows.go`,
`detach_test.go`, `hostlock.go`; `driver/hyperv/freshness.go`,
`prepare.go` (`activePrepared`, `prepareLockName`), `buildassets.go`
(`hypervCacheDir`), `main_test.go`, `prepare_test.go` (`withBase`);
`driver/qemu/driver.go` (`resolveWindowsBaseImage`,
`ensureFreshWindowsLayer`), `prepare.go` (`prepareLockName`, `prepareFix`),
`main_test.go`; the Hyper-V and qemu Windows paragraphs of
`docs/book/src/guide/drivers.md`; the `"hyperv-prepare"` line of
`overview.md`. This replaces Design's eight-step outline. One PR, one
`f3-implement` step. Planning found the design workable as written and
sends nothing back.

Tasks 1, 3 and 5 are the rework of code `main` has (FLOW-06):
`StartDetached`'s body (from `project-config-trust` and
`hyperv-windows-prepare`), and the two callers' dropped-error branches.
Nothing else already implemented changes; `detachedArgs`, `detachedAttr`,
`ConfigFile` and `TestDetachedArgs` are untouched. Tasks 8 and 9 are the
reference doc and `overview.md` (UX-01, FLOW-15). There is no rule change
and so no rule task (FLOW-14). Tier 0 lands with the task whose behaviour
it tests; it stays pure Go with no network, no VM and no container
runtime, and each test is deterministic: a wait on the child polls for
its output to a deadline and fails at it, never retries a failed run
(TEST-01, TEST-04). Task 10 is tier 1, task 11 the push; tier 2 is the
existing `f6-validate-windows-hyperv` step and the tracked gap
`detached-prepare-windows-exe-qemu` (TEST-02, TEST-03, FLOW-09). The code
is in `driver/` (one shared file and its tests) and in the two callers in
`driver/hyperv` and `driver/qemu`; no other driver tree changes, and no
build-tagged file is added or edited (ORCH-01, GLOBAL-07).

*r2 (2026-11-18).* `f3-implement` returned the design (FLOW-06): tier 1's
`drivers-claim` failed on task 1 alone, because `driver/detach.go` is in
`rules.yaml`'s `drivers.shared` and the design declared `[qemu, hyperv]`.
The design now declares every driver (a5). No task changes: tasks 1 and 2,
written and not pushed, are redone as planned, and task 10's `make lint`
and `GOOS=windows make lint` now pass `drivers-claim` with no trailer. The
areas are unchanged: `driver-core` is a shared area, which covers every
declared driver (FLOW-02).

*Plan r2 (2026-11-19).* Confirmed against the design as its r2 review
approved it (**approved at `0c1cad5`**, a5 answered (a)) and against
`origin/main` `e9315c2`, merged into the branch (FLOW-10). `main`'s new
commits (#127 to #129) change only the loop, its process text and
`tools/workflow/`; none touches `driver/`, `docs/book/` or
`tools/tier1/`, so every file the tasks read or edit is as this plan
read it, and every rule a task cites stands. The eleven tasks and their
numbers stand. Tasks 1 and 2 start from `main`'s code: the earlier
attempt was never committed and is gone, so `driver/detach.go` still
starts through `exec.Command` and `driver/main_test.go` does not exist.
The `drivers` change asks nothing more of a task: container, vz and
firecracker are covered by task 10's tier 0 and tier 1 and the
`integration` workflow, with no native step (Cross-driver impact, a5;
TEST-02), and the native steps are unchanged.

Tasks 1 and 2 are one green checkpoint, as are 3 and 4, and 5 and 6.

**Plan notes.** Where the design is silent on how a thing is done, the
plan takes the reading below; none changes a decision, a behaviour, an
impact field or a rule. They are for the Review, and for Design only if
a reading is wrong.

*Note A (the seams).* The design's tier 0 injects the start (callers) and
starts from a path that does not exist (`StartDetached`). Reading: in
`driver/`, a package variable `executable = os.Executable` that only tests
set; in each caller package, a package variable `startDetached =
driver.StartDetached`, and in `driver/qemu` also `tryHostLock =
driver.TryHostLock` for the lock-error case. No exported API is added.

*Note B (qemu's progress stream).* The qemu caller's existing line goes to
`os.Stderr`, which is that driver's progress stream (`(*Driver).status`).
Reading: `ensureFreshWindowsLayer` takes a `progress io.Writer`, as
`ensureFreshBase` does, and `resolveWindowsBaseImage` passes `os.Stderr`,
so both lines reach the stream the existing one does and the test reads
them from a buffer.

*Note C (the wording).* The new line is the design's (Design, the two
pseudocode blocks), one line, with `%v` of the error:
"[warden] prepared base is <age> old, and its background refresh did not
start: <err>; run \`warden image prepare --driver hyperv\` to refresh it";
qemu's says "prepared Windows layer" and names `prepareFix(ga)`. The age
is `winguest.RoundAge` as in the existing line; the hyperv fix is the
`prepareFix` constant, so neither command is spelled twice (GLOBAL-01).
The lock-error line is the same line with the lock's error.

### Tasks

1. **Rework `StartDetached` (`driver/detach.go`).** Per Decision 1 and the
   Design pseudocode: `exe` from `executable()`; `MkdirAll` and open the
   log as today; a new unexported `startDetached(exe, log, args)` opens
   `os.DevNull`, calls `os.StartProcess(exe, append([]string{exe},
   args...), &os.ProcAttr{Files: {null, log, log}, Env: os.Environ(), Sys:
   detachedAttr()})` and releases the process, closing the null file
   either way. On a failed start, `StartDetached` writes `[warden] could
   not start <exe>: <err>` to the open log and returns the error. Drop the
   `os/exec` import; update the doc comment to say the named file is run
   with no lookup (Decision 1; Note A). Rules: GLOBAL-01, GLOBAL-07,
   GLOBAL-10, GLOBAL-11, ORCH-05, SEC-01.
2. **Tier 0 for task 1 (`driver/detach_test.go`, and a `TestMain` in a new
   `driver/main_test.go`).** `TestMain` runs a helper mode when an
   environment variable the test sets (e.g. `WARDEN_TEST_DETACHED_HELPER`)
   is present: it prints its own `os.Executable()` and exits 0, before
   `m.Run`. Cases (Validation plan, Tier 0):
   - `TestStartDetachedRunsNamedFile`: the test binary copied to
     `<dir>/warden` (no extension, mode 0755) and to `<dir>/warden.exe`;
     `startDetached` on `<dir>/warden` with the helper variable set; poll
     the log to a deadline; it names `<dir>/warden` (compared after
     `filepath.EvalSymlinks` of both, for temp dirs behind a link). On
     Linux the `.exe` copy is inert; on Windows this is native case 1
     (TEST-08's unit form).
   - `TestStartDetachedFailedStart`: `executable` set to a path that does
     not exist; `StartDetached` returns an error and the log holds
     `could not start <path>`.
   - `TestDetachedArgs`, unchanged.
   Tier 0 (TEST-02). Rules: TEST-01, TEST-02, TEST-04, TEST-08.
3. **Rework `ensureFreshBase` (`driver/hyperv/freshness.go`).** Through
   `startDetached` (Note A); on success the existing line, unchanged; on
   failure the new line (Note C) on `progress`; the function returns nil
   either way, so the build goes on on the current base (Decision 2, a2).
   `prepareRunning` is unchanged. Rules: GLOBAL-11, UX-04, ORCH-01.
4. **Tier 0 for task 3 (`driver/hyperv/freshness_test.go`, new).** A
   hermetic cache with an active prepared base older than a 1m
   `ImageRefreshAfter` (from `withBase` and the manifest helpers
   `prepare_test.go` uses), `startDetached` replaced by one returning an
   error: `ensureFreshBase` returns nil and the buffer holds exactly the
   new line with the error and `warden image prepare --driver hyperv`; with
   one returning nil, the existing "refreshing it in the background" line
   and the args `image prepare --driver hyperv`. Untagged, so it runs on
   Linux CI and on the Windows host (native case 1). Tier 0 (TEST-02).
   Rules: TEST-01, TEST-02, TEST-04.
5. **Rework `ensureFreshWindowsLayer` (`driver/qemu/driver.go`).** Takes
   `progress` (Note B); a `tryHostLock` error prints the new line with that
   error and returns nil (a held lock stays silent, as today); a failed
   `startDetached` prints the new line; success prints the existing line,
   unchanged (Decision 2, a2; Design, the qemu wording). Rules: GLOBAL-11,
   UX-04, ORCH-01.
6. **Tier 0 for task 5 (`driver/qemu/windows_test.go` or a new
   `freshness_test.go`).** A `PreparedLayer` whose `PreparedAt` is past a
   1m `ImageRefreshAfter`, with grace left: with `startDetached` failing,
   with `tryHostLock` failing, and with both succeeding, the function
   returns nil and writes, in turn, the new line naming the start error
   and `prepareFix(ga)`, the new line naming the lock error, and the
   existing line. Tier 0 (TEST-02). Rules: TEST-01, TEST-02, TEST-04.
7. **Check that no other site re-executes warden through `os/exec`**
   (Prior art): `grep` for `os.Executable()` and `exec.Command(exe` across
   the tree on the branch; record the result in the PR. No edit expected;
   a hit is a design gap and goes back to Design (FLOW-06). Rules: GLOBAL-10,
   FLOW-06.
8. **`docs/book/src/guide/drivers.md`.** In the Hyper-V paragraph on the
   background re-prepare: a refresh that cannot start is reported on the
   build's output with the command to run by hand, and the build goes on;
   the qemu Windows sentence ("see the Hyper-V section ...") already points
   there and gains "and the same report". Rules: UX-01.
9. **`docs/book/src/concepts/overview.md`.** The `"hyperv-prepare"` lock
   line gains "a refresh that does not start is reported, the build goes
   on"; nothing else in the hyperv or qemu flows changes (Design, Docs).
   Rules: FLOW-15.
10. **Tier 1 and the gate.** `make test`, `make lint`, and `GOOS=windows
    make lint` (`driver/hyperv` builds its real driver only there; the
    Windows `detachedAttr` is compiled under it). No new CI job
    (Decision 4). The `integration` workflow runs on the PR because it
    reaches the `driver/` root; it must pass. Tier 0 and tier 1
    (TEST-02). Rules: TEST-01, TEST-02, FLOW-17.
11. **Push and open the PR to `main`.** Push the branch at each green
    checkpoint (tasks 2, 4, 6, 10); the PR body names the blocking
    `windows-hyperv` step and the tracked gap
    `detached-prepare-windows-exe-qemu`, and lists the task-7 grep. Rules:
    FLOW-08, FLOW-09, FLOW-16, FLOW-17.

Every decision is carried by a task: Decision 1 by 1 and 2, Decision 2 by
3 to 6, Decision 3 by no edit to the `Makefile` (no task touches it),
Decision 4 by task 10's "no new CI job". The Security note's TEST-08
case is task 2's first case and native case 3; the release's security
suite is TEST-08's, not this item's. a3 is native case 3. No task changes
`make build`, `.claude/`, `docs/development/` or `tools/`.

## Design review

- **r1, 2026-11-17: approved at `b7619b5`** (Jeff, workflow-human, FLOW-19).
  Assumptions a1 (`make build` keeps `dist/warden`), a2 (a due refresh that
  cannot start warns, logs and the build goes on), a3 (the stale
  `dist/warden.exe` is case 3's file beside, then deleted) and a4 (exempt
  from FLOW-20 as a fix for a regression on `main`, the follow-up of a
  security fix) confirmed as drafted, (a) each. No `rule_changes`; no changes
  folded in. Areas as declared; the area-edges check showed no difference
  from `main` at 0cd3c02.
- **r2, 2026-11-18: approved at `0c1cad5`** (Jeff, workflow-human, FLOW-19).
  a5 (flagged) answered (a), as recommended. A delta re-review (FLOW-19), of the `drivers`
  field only. Finding (`f3-implement`, FLOW-06): tier 1's ORCH-01
  `drivers-claim` fails because task 1 edits `driver/detach.go`, which
  `tools/tier1/rules.yaml` counts as `drivers.shared`, while the design
  approved at `b7619b5` declared `drivers: [qemu, hyperv]`. What it changes:
  `drivers` becomes `[container, qemu, vz, hyperv, firecracker]` (a5),
  Cross-driver impact states the claim and keeps container, vz and
  firecracker as asserted unaffected, and `native_validation` is
  unchanged. Every other impact field, `repos`, `areas`, `rule_changes`,
  the decisions, a1 to a4 and the tasks are as approved; the delta to
  review is the r2 commit (front-matter `drivers` and `design_review`,
  a5, Prior art's precedent line, Cross-driver impact, the plan's r2
  paragraph and this line). No changes folded in.
  sitting: about 1 minute, asks: 1.

## Consolidation

## Rollback notes

## Archive summary
