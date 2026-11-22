---
feature: config-lookup-test-symlinked-tmp
status: draft
drivers: []              # cmd/warden test files only; no driver tree, and no shared
                         # package (driver/, relay/, ledger/, collector/) is edited
seams: []
surfaces: []             # no change to what warden prints, reads or accepts: the config:
                         # line keeps printing the working directory as the process got it
                         # (its PWD when that names the directory), which is the shell's view
repos: []                # only buildwarden/buildwarden
areas: [cli, designs]    # cmd/warden/vmdns_test.go (inConfigFiles), configline_test.go, and
                         # one new test in configfile_test.go; this design
security_boundary: none  # test code only; which file warden reads, and the line that says
                         # so, are unchanged
alters_relationships: false
design_review: required  # FLOW-19: the queue records no areas (intake took none from
                         # project-config-trust), so declaring [cli, designs] narrows it,
                         # which only a review may agree to; and q1 is assumed
native_validation:
  - {host: qemu-any, blocking: false, reason: "no host's driver or lifecycle is touched: the change is in cmd/warden's tests, and the behaviour they pin (os.Getwd taking PWD when it names the working directory) is the same Go code on every Unix; the new tier-0 test reproduces macOS's symlinked temp directory on Linux CI. What only the macbook adds is the run that failed: go test ./cmd/warden on darwin/arm64, which CI does not run"}
tier2_gaps:
  - {host: qemu-any, gap: "on the macbook (darwin/arm64), on the branch and then on main after the merge: go test -count=1 ./cmd/warden passes, TestConfigFileLookupOrder included, with t.TempDir under /var/folders (a link into /private/var); and in a shell, cd /var/folders/<...>/<dir> holding a warden.toml, warden doctor's first line names /var/folders/<...>/warden.toml, not /private/var/... (backlog config-lookup-test-symlinked-tmp-darwin)"}
rules: [TEST-01, TEST-02, TEST-03, TEST-04, GLOBAL-07, GLOBAL-09, GLOBAL-10, GLOBAL-11,
        UX-01, UX-07, FLOW-01, FLOW-02, FLOW-05, FLOW-07, FLOW-09, FLOW-12, FLOW-19,
        FLOW-20, LOOP-04]
rule_changes: []
concepts: [config-file, config-lookup, config-line, working-directory, symlinked-tmp,
           getwd, pwd, macos, testing, test-fix, regression-fix]
related: [project-config-trust.md, jks-test-skip-without-jdk.md]
---

# The config lookup tests change directory the way a shell does

## Problem

`go test -count=1 -run TestConfigFileLookupOrder ./cmd/warden` fails on
darwin/arm64, on `main` at `e9315c2` and again at `be13847` (the macbook's
`qemu-any` run of 2026-11-20, log
`native/qemu-any/2026-11-20/cmd-warden-go-test.log` on the `workflow`
branch). Two subtests, "./warden.toml when nothing names a file" and "an
empty variable is not set", report

```
config file = "/private/var/folders/.../002/warden.toml" ("found in the working directory"),
want          "/var/folders/.../002/warden.toml" ("found in the working directory")
```

at `cmd/warden/configfile_test.go:125`, a test `project-config-trust` added.
So `make test` fails on macOS for everyone, against TEST-01 ("runs on every
supported platform"), and every done-when that runs it there fails with it.
CI did not catch it: CI runs `go test ./...` on Linux only, and its macOS leg
(`tier1-test`) runs only `tools/tier1` (`jks-test-skip-without-jdk`, Prior
art).

**Cause.** On macOS `t.TempDir()` is under `$TMPDIR`, `/var/folders/...`,
and `/var` is a symbolic link to `/private/var`. The test helper
`inConfigFiles` (`cmd/warden/vmdns_test.go`) enters the temporary directory
with `os.Chdir`, which does not set `PWD`, and returns the path as
`t.TempDir` gave it. `configFile` (`cmd/warden/config.go`) makes
`./warden.toml` absolute with `filepath.Abs`, which calls `os.Getwd`. On
every Unix `os.Getwd` returns `$PWD` when `$PWD` is absolute and names the
current directory, and otherwise asks the kernel, which gives the path with
every link resolved. In the test `$PWD` is still the package's directory, so
`os.Getwd` asks the kernel and gets `/private/var/...`, while the test
expects the unresolved `/var/...`.

**Reproduced on Linux** at `be13847` with `TMPDIR` set to a symbolic link to
a real directory: the same two subtests fail with the same mismatch, and no
other test in `./...` fails for the link (one firecracker test failed in
that run only because the sandbox's temp parent was not traversable by
`nobody`; it passes with the default `TMPDIR` and has no link in it).

**What a user sees today.** Built from `be13847` and run from a shell in a
directory reached through a link (`cd <link>/proj`, so the shell's `PWD` is
`<link>/proj`), `warden doctor` prints

```
config: <link>/proj/warden.toml (found in the working directory)
```

the directory as the shell shows it. Only with `PWD` unset or stale (a
process that changed directory without updating it) does it print the
resolved `<real>/proj/warden.toml`. The product does what a user expects;
the test does not call it the way a shell does.

This is the follow-up of `project-config-trust` (FLOW-09: a regression
`main` already has, found while validating). FLOW-20's freeze exempts it as
a fix for a regression on `main`.

## Prior art

- **`project-config-trust`** (archived; the item this follows) designed the
  lookup and the `config:` line: "A path is printed absolute, whatever form
  it was given in, and escaped" (its section "What says which file was
  read"). It says nothing of links in the working directory's path; its
  code makes the path absolute with `filepath.Abs`, which is what this
  design keeps. Its tests `TestConfigFileLookupOrder`,
  `TestConfigLineEscapesThePath` and the helper `inConfigFiles` are the
  ones changed here.
- **`jks-test-skip-without-jdk`** (archived) is the earlier test-only fix
  for a `make test` failure only the macbook saw, and records that CI runs
  no `cmd/` tests on macOS. Its review was skipped; this one cannot be (see
  `design_review` in the front-matter, and Decision 3).
- **`TestResolvePath_Empty`** (`cmd/warden/config_test.go`) meets the same
  link by resolving the expected path with `filepath.EvalSymlinks` and
  entering the resolved directory. That fits a function whose result is a
  build context, compared by identity; it does not fit the `config:` line,
  whose text is what a user reads (Decision 2, alternative b).
- **Go's `testing.T.Chdir`** (Go 1.24; `go.mod` is at 1.26.3) changes
  directory, sets `PWD` to the new directory on every system but Windows and
  Plan 9, and restores both when the test ends. It refuses a parallel test,
  as `t.Setenv`, which these tests already call, does.
- **The search** of `docs/design/` front-matter for `config-lookup`,
  `config-file`, `working-directory`, `symlink` and `getwd` found only
  `project-config-trust` (and `runtime-cli-project-path`, which it
  supersedes on the config file and which does not touch the working
  directory's path).

## Decisions

1. **The test is wrong; the product is right.** For `./warden.toml` the
   `config:` line names the working directory as the process was given it:
   the shell's `PWD`, through `os.Getwd`, which is what the user typed
   `cd` to and what their prompt shows. Least surprise (GLOBAL-09) is the
   path the user sees, not one with `/private` put in front of it. And it
   is what `warden` prints today wherever a shell starts it, so fixing the
   test keeps the user-facing line as it is: no surface changes (UX-01 has
   nothing to update), and nothing in GLOBAL-10 or GLOBAL-11 is touched.
   Alternatives:
   - (a) *Resolve the path in `configFile` (`filepath.EvalSymlinks`) so the
     line always names the real file.* Rejected: it changes what every
     macOS user reads for the common case, `/var/...` and any linked home
     or project directory becoming a path they never typed; it is a change
     to an output surface, made to suit a test; and a link the line resolves
     still is not a security property (the file read is the same either
     way, and the line already says which file was read).
   - (b) *Resolve the expected path in the test* (`EvalSymlinks` on `wd`,
     as `TestResolvePath_Empty` does). Rejected: it pins the resolved path,
     which a user sees only when `PWD` is missing or stale, so the test
     would assert the uncommon case and pass for the wrong reason, and
     would fail if `os.Getwd` ever ignored a stale `PWD` differently.
   - (c) *Build the expected path from `os.Getwd()` in the test.* Rejected:
     it compares `filepath.Abs` with the call `filepath.Abs` makes, so it
     asserts nothing about the path a user reads.
2. **`inConfigFiles` enters its directory with `t.Chdir`**, which sets
   `PWD` as a shell does, so the tests run `warden`'s lookup the way a user
   starts it and expect the directory as `t.TempDir` named it. This is the
   smallest correct change: one helper, and one more `os.Chdir` in
   `TestConfigLineEscapesThePath`, which enters a subdirectory after
   `inConfigFiles` and, with `PWD` now set to the parent, would otherwise
   leave `PWD` stale and fail the same way (seen on Linux with a linked
   `TMPDIR` when only the helper was changed). `t.Chdir` also restores the
   directory through a held descriptor rather than a path, which removes the
   helper's own `os.Getwd` and its ignored error.
3. **A test that links its directory on every OS.** A new
   `TestConfigLineNamesTheDirectoryAsEntered` in `configfile_test.go` makes
   a directory with a `warden.toml`, a symbolic link to it, enters the link
   with `t.Chdir`, and expects `cfg.File`, and the `config:` line, to name
   the link's path. It skips on Windows (no symbolic links without
   privilege, and Windows does not use `PWD`; `selfLink` skips the same
   way). This puts macOS's case in Linux CI, so a change that starts
   resolving the path, or a helper that loses `PWD` again, fails on every
   push and not only on the macbook. Rejected: relying on the macbook's
   native runs alone (the reason this reached `main`).
4. **`design_review: required`.** The change is test-only and qualifies on
   every FLOW-19 criterion but one: the queue records no `areas` (intake
   took them from `project-config-trust`, which had none), and declaring
   `[cli, designs]` where the queue records none counts as fewer, which only
   a review may agree to. Rejected: leaving `areas` out to keep the review
   skippable, which would declare the item as colliding with every item in
   the repository (FLOW-05) to avoid a short review, and would not be the
   honest declaration FLOW-02 asks for. q1 is also assumed, and needs the
   review either way.

## Questions

### Blocking

None.

### Assumed

- **q1. Fix the test, not the product: for `./warden.toml` the `config:`
  line keeps naming the working directory as the process was given it (the
  shell's `PWD`), not as links resolve it.** Options: (a) fix the test so it
  enters its directory as a shell does (`t.Chdir`), and add a test that
  pins the unresolved path on every OS; (b) resolve the path in
  `configFile`, so the line always names the real file, and update the
  tests and the reference; (c) resolve the expected path in the test only.
  Recommended: (a), Decision 1. **Confirmed (a), Jeff, 2026-11-21.**

## Design

Only `cmd/warden`'s tests change; `cmd/warden/config.go` and everything a
user reads are unchanged. Only this repository is touched (LOOP-04).

- `inConfigFiles` (`cmd/warden/vmdns_test.go`): replace

  ```go
  orig, _ := os.Getwd()
  if err := os.Chdir(dir); err != nil { t.Fatal(err) }
  t.Cleanup(func() { _ = os.Chdir(orig) })
  ```

  with `t.Chdir(dir)`, and say in its comment that it enters the directory
  as a shell does, setting `PWD`, so the paths `warden` prints are the
  directory as `t.TempDir` named it.
- `TestConfigLineEscapesThePath` (`cmd/warden/configline_test.go`): its
  `setup` enters the escaped subdirectory with `t.Chdir(dir)` instead of
  `os.Chdir(dir)`.
- `TestConfigLineNamesTheDirectoryAsEntered` (new, `configfile_test.go`):
  skip on Windows; `inConfigFiles(t, "")`; `real := t.TempDir()` with a
  `warden.toml` setting `output_dir`; `link := filepath.Join(t.TempDir(),
  "linked")`, `os.Symlink(real, link)`; `t.Chdir(link)`; `LoadConfig`;
  expect `cfg.File == filepath.Join(link, "warden.toml")`, the source
  `found in the working directory`, the output directory from the file, and
  `configLine(cfg)` naming the same path.
- `TestConfigFileLookupOrder` is unchanged: with the helper fixed, its
  expectation (the directory `inConfigFiles` returned) is the right one.

The other callers of `inConfigFiles` (in `buildlink_test.go`,
`config_former_test.go`, `config_test.go`, `configline_test.go`,
`configmsg_test.go`, `precedence_test.go`, `relayimage_test.go`,
`runtime_test.go`, `upstreamca_test.go`, `upstreamca_windows_test.go`,
`vmdns_test.go`, and the rest of `configfile_test.go`, `TestRelativeConfigFile`
among them) take their expected paths from the helper's return or
from `configPath(t)` (`os.Getwd`), or compare no path, and both agree once
`PWD` is set; the package passes with the change, with the
default `TMPDIR` and with a linked one (checked at `be13847` on Linux while
designing; the change was then reverted, it is Implement's).

No area or link is missing from the area map: `cli` holds `cmd/warden/`.

## Defaults and overrides

None. The design adds and changes no default, override, merge or fallback.
For the record, the behaviour the tests now pin is an existing one of Go's
`os.Getwd` on Unix: `$PWD` when it is absolute and names the current
directory, else the kernel's resolved path. It is not a fallback `warden`
adds (GLOBAL-11): both give the same directory, and the line names the file
that was read either way.

## Cross-driver impact

None: no driver's code, and no shared package, is edited. `drivers` is
empty.

## Validation plan

- **Tier 0** (`make test`, Linux in CI): the package passes; the new test
  fails if `configFile` resolves links or if `inConfigFiles` stops setting
  `PWD`. Implement also runs `TMPDIR=<a link to a directory> go test
  -count=1 ./cmd/warden` once and records it in the commit message: it
  failed at `be13847` and must pass on the branch.
- **Tier 1**: `make lint` (no build-tagged file is touched).
- **Tier 2, tracked, `qemu-any`** (the macbook, which found it): `go test
  -count=1 ./cmd/warden` on darwin/arm64, and `warden doctor` from a
  directory under `/var/folders` naming `/var/...`. Tracked, not blocking
  (FLOW-09): the change reaches no driver or lifecycle of that host, and the
  tier-0 test reproduces its case on Linux. Recorded in `tier2_gaps` and as
  `config-lookup-test-symlinked-tmp-darwin` on the native-gap backlog; the
  macbook's next sweep tests the branch with `main`.

## Implementation plan

Plan from the design as approved at `8eb333d` (r1). Only `cmd/warden`'s test
files change; no package in `cmd/warden` calls `t.Parallel`, so `t.Chdir`
cannot panic in any caller of `inConfigFiles`.

1. **`inConfigFiles`** (`cmd/warden/vmdns_test.go`): replace the
   `os.Getwd`/`os.Chdir`/`t.Cleanup` lines with `t.Chdir(dir)`, and say in
   its comment that it enters the directory as a shell does, setting `PWD`,
   so the paths `warden` prints are the directory as `t.TempDir` named it
   (Decision 2; TEST-01, TEST-04). Rework of `project-config-trust`'s
   helper (FLOW-06). `TestConfigFileLookupOrder` and the helper's other
   callers are not edited (Design).
2. **`TestConfigLineEscapesThePath`** (`cmd/warden/configline_test.go`): its
   `setup` enters the escaped subdirectory with `t.Chdir(dir)` in place of
   `os.Chdir(dir)` and its error check (Decision 2; TEST-01, TEST-04).
   Rework (FLOW-06).
3. **Tier 0: add `TestConfigLineNamesTheDirectoryAsEntered`**
   (`cmd/warden/configfile_test.go`), as the Design section lays it out:
   skip on Windows with `selfLink`'s reason; `inConfigFiles(t, "")`; a real
   directory holding a `warden.toml` that sets `output_dir`; a symbolic link
   to it in another `t.TempDir()`; `t.Chdir(link)`; `LoadConfig`; expect
   `cfg.File == filepath.Join(link, "warden.toml")`, the source "found in
   the working directory", the file's output directory, and
   `configLine(cfg)` naming the same path (Decision 3; TEST-01, TEST-02,
   TEST-03, TEST-04, GLOBAL-07, GLOBAL-09, UX-07). Confirm the Validation
   plan's claim once: with `configFile` made to resolve links (a local edit,
   never committed) the test fails; then revert the edit.
4. **Tier 0 and tier 1 gates**: `make test` and `make lint` pass (no
   build-tagged file is touched, so the host platform's lint is the one
   owed; FLOW-17); then `TMPDIR=<a link to a directory> go test -count=1
   ./cmd/warden` passes, and the commit message records both that run and
   that it failed at `be13847` (Validation plan; TEST-01, TEST-02, TEST-04).
5. **Tier 2, tracked, not a step**: the darwin/arm64 run and the
   `warden doctor` check under `/var/folders` stay the `qemu-any` gap in
   `tier2_gaps`, already on the native-gap backlog as
   `config-lookup-test-symlinked-tmp-darwin`; Implement adds no
   `f6-validate-*` step and edits no backlog entry (TEST-03, FLOW-09).

No product code, reference doc (`docs/book/src/guide/`) or pseudocode in
`overview.md` changes: no surface changes and no component's flow changes
(UX-01, FLOW-15). No rule change was approved and none is needed
(`rule_changes: []`; FLOW-14).

## Design review

- **r1, 2026-11-21: approved at `8eb333d`** (Jeff, workflow-human, FLOW-19).
  Repositories: this one only (LOOP-04); areas [cli, designs] confirmed, which
  narrows the queue's record from undeclared. q1 confirmed (a). No rule change
  and no specification change. Nothing folded in.
  - sitting: 19:05-19:08 UTC, asks: 3 (0 in batches)

## Consolidation

Publish, 2026-11-22 (FLOW-10, FLOW-15), on the branch at `f7ff696`.
Forked from `main` at `be13847`; `origin/main` is still `be13847`, so
`f6-merge-main` had nothing to merge and the branch contains `main` as it
is.

- **What landed on `main` since the fork.** Nothing: `origin/main` at
  `be13847` is the merge base. Nothing to reconcile.
- **Terminology and wording.** The Design section named six of the twelve test
  files that call `inConfigFiles`. The list now names every one, and says
  that some compare no path. The claim it makes
  is unchanged and holds for each: every caller takes its expected path
  from the helper's return or from `configPath(t)` (`os.Getwd`), or
  compares none. No term in the design differs from `project-config-trust`'s
  (`config:` line, "found in the working directory", `./warden.toml`).
- **FLOW-15 sweep.**
  - `make deadcode`: the unreachable functions it lists are all in
    `cmd/relay`, `collector`, `driver/firecracker`, `driver/hyperv`,
    `driver/vz` and `relay`, none touched here; the branch changes only
    `_test.go` files in `cmd/warden` and adds no helper.
  - Duplication: the new test reuses `inConfigFiles`, `write`,
    `outputDirFile`, `configLine` and `selfLink`'s Windows skip reason, and
    adds no helper. The one other `os.Chdir` in the repository's tests,
    `TestResolvePath_Empty` (`cmd/warden/config_test.go`), resolves its
    path with `EvalSymlinks` and compares a build context by identity, as
    Prior art records; it is not a caller of the lookup and is left as it
    is.
  - `docs/book/src/concepts/overview.md`: `warden build`'s config lookup
    (`--config`, else `WARDEN_CONFIG`, else `./warden.toml`) is unchanged
    and no component's flow changes, so it is unchanged.
- **Seams.** `seams: []`; no specification changes, and no
  **Specification changes** section is needed (LOOP-02).
- **Rules proposed:** none. **Follow-ups filed:** none. **Regressions
  `main` already has:** none found beyond the one this item fixes.
- **`dist/wf check` on `f7ff696`**, its trial-merge lines:

  ```
  ok   trial merge with workdir-in-context (a report, FLOW-10): clean
  ok   trial merge with relay-collector-failure-fails-open (a report, FLOW-10): clean
  ```

- **Gates:** `make lint` and `make test` pass on the consolidation commit;
  `TMPDIR=<a link to a directory> go test -count=1 ./cmd/warden` passes.

## Rollback notes

## Archive summary
