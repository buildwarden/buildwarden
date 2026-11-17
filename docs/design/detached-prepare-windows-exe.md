---
feature: detached-prepare-windows-exe
status: draft
drivers: [qemu, hyperv]  # conservatively (FLOW-02): the change is in driver/detach.go, shared, and its
                         # dependents are the two drivers that call driver.StartDetached (the hyperv
                         # base refresh, driver/hyperv/freshness.go, and the qemu Windows layer
                         # refresh, driver/qemu/driver.go). container, vz and firecracker do not call
                         # it (Cross-driver impact)
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
                         # questions, and areas declared where intake recorded none
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
  Archive summary files this item.
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

- **hyperv:** `ensureFreshBase` reports a failed start. Its refresh now
  starts from a `dist/warden` with no extension.
- **qemu:** `ensureFreshWindowsLayer` reports a failed start and a lock
  error. On macOS and Linux the start is the same `exec` of the same path.
- **container, vz, firecracker:** unaffected. None calls `StartDetached`;
  `vz` refreshes its image only through `sudo warden image prepare`, by
  hand.

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

1. `driver/detach.go`: `StartDetached` through `os.StartProcess` with the
   named file, stdin the null device, stdout and stderr the log, the
   inherited environment, `detachedAttr()`; on a failed start the error is
   written to the log and returned (Decision 1; GLOBAL-01, GLOBAL-07,
   GLOBAL-10, GLOBAL-11).
2. `driver/detach_test.go`: the tier-0 cases, the helper mode selected by
   an environment variable the test sets (TEST-01, TEST-04).
3. `driver/hyperv/freshness.go`: report a failed start with the fix, the
   build going on; the start function injectable for the test (Decision
   2; UX-04, GLOBAL-11, ORCH-01).
4. `driver/qemu/driver.go`: the same for `ensureFreshWindowsLayer`, and the
   `TryHostLock` error (Decision 2; UX-04, ORCH-01).
5. Tests for 3 and 4 in their packages.
6. `docs/book/src/guide/drivers.md`: the Hyper-V refresh paragraph, and
   the qemu Windows sentence that refers to it (UX-01).
7. `docs/book/src/concepts/overview.md`: the `"hyperv-prepare"` line
   (FLOW-15).
8. `make test`, `make lint`, `GOOS=windows make lint`; push (FLOW-17).

No rework of other items' code beyond `StartDetached`'s body and the two
callers' error branches.

## Design review

- **r1, 2026-11-17: approved at `b7619b5`** (Jeff, workflow-human, FLOW-19).
  Assumptions a1 (`make build` keeps `dist/warden`), a2 (a due refresh that
  cannot start warns, logs and the build goes on), a3 (the stale
  `dist/warden.exe` is case 3's file beside, then deleted) and a4 (exempt
  from FLOW-20 as a fix for a regression on `main`, the follow-up of a
  security fix) confirmed as drafted, (a) each. No `rule_changes`; no changes
  folded in. Areas as declared; the area-edges check showed no difference
  from `main` at 0cd3c02.

## Consolidation

## Rollback notes

## Archive summary
