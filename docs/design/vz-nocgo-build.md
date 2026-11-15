---
feature: vz-nocgo-build
status: draft
drivers: [vz]            # driver/vz only: the build constraints of vz_stub.go and vz_darwin_arm64.go, and checkPlatform in driver.go; no shared driver/ package, so no other driver is reached (FLOW-02)
seams: []                # no versioned seam: which file of driver/vz a build compiles is inside the driver
surfaces: [output]       # the error a warden built without cgo gives for the vz driver (UX-04); no flag, subcommand or config key changes
security_boundary: none  # a build that has no vz driver starts no VM and no relay; the cgo build compiles the files it compiles today
alters_relationships: false  # inside the vz driver's existing role: which of its two platform files a build compiles
design_review: required  # blocking native validation on macos-vz (FLOW-19)
native_validation:
  - {host: macos-vz, blocking: true, reason: "the change is in driver/vz (FLOW-09): it decides which of the driver's two platform files a build on macOS/arm64 compiles. Only that host can show that the cgo build still has the driver and runs a vz build, and that a warden built there without cgo gives the message for doctor, build and image prepare"}
tier2_gaps: []
rules: [FLOW-01, FLOW-02, FLOW-09, FLOW-14, FLOW-17, FLOW-19, GLOBAL-01, GLOBAL-04, GLOBAL-07, GLOBAL-10, GLOBAL-11, ORCH-01, PROFILE-01, TEST-01, TEST-02, TEST-03, UX-01, UX-04, UX-09]
rule_changes: []
concepts: [vz, cgo, build-constraints, platform-stub, release-build, goreleaser, cross-build, ci-build-job, fixable-error]
related: [vm-driver-release-plan.md, vz-image-prepare-refresh.md, ci-path-filters.md, rule-running-line-match.md]
---

# The macOS arm64 `warden` compiles without cgo, and says it has no vz driver

## Problem

`GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/warden` fails on `main`
(`3cb0b2b`):

```
driver/vz/vm.go:25:22: undefined: linuxVMConfig
driver/vz/vm.go:28:22: undefined: macOSVMConfig
driver/vz/vm.go:33:17: undefined: createLinuxVM
```

`driver/vz` keeps its Virtualization.framework bindings in
`vz_darwin_arm64.go` (`//go:build darwin && arm64`, and `import "C"`) and
their stand-ins in `vz_stub.go` (`//go:build !darwin || !arm64`). A file that
imports `"C"` is left out of every build with cgo off, whatever its
constraint says. So on darwin/arm64 with cgo off the bindings are left out
by cgo and the stub by its constraint, and the types and functions `vm.go`
uses are defined nowhere. Every other target compiles one of the two.

`.goreleaser.yaml` builds `warden` for linux and darwin, amd64 and arm64,
with `CGO_ENABLED=0`, on an Ubuntu runner. One of its four builds cannot
compile, so no release can be cut. Nothing has shown it, because the only
release attempted since the vz driver landed, v0.6.0 (run 41251092199,
2026-10-31), stopped earlier, in the `before` hook. The hook's two faults
are `prebuilt-artifacts`' to fix (its Decision 12); this one was found by
that design and filed on its own, to start now (Jeff, 2026-11-15). It is a
regression `main` already has, exempt from the freeze (FLOW-09, FLOW-20).

The same build is what anyone gets who cross-builds for an Apple Silicon Mac
from another machine, where cgo is off by default: `GOOS=darwin GOARCH=arm64
golangci-lint run ./driver/vz/` fails the same way on Linux.

### When and how it broke

- **The two constraints were never complements.** They are as `312e2f4`
  (2026-05-13, "Add driver interface, relay library, and vz driver
  skeleton", on the `vm-drivers` branch) wrote them, and no commit has
  changed either since.
- **`main` got the fault with PR #52** (`89f1e25`, 2026-06-01, the merge of
  `vm-drivers`), which brought `driver/vz` and `cmd/warden`'s import of it.
  Built for this design: `cmd/warden` for darwin/arm64 without cgo compiles
  at the merge's first parent (`8228503`) and fails at `89f1e25` with the
  errors above.
- **No release has held a vz driver or its stub.** v0.1.0 to v0.5.0 (the
  last published, 2026-05-22) were built from trees with no `driver/vz`.
  Their `.goreleaser.yaml` already built `warden` for darwin with
  `CGO_ENABLED=0`; that line has not changed.

### Why CI did not catch it

- `main.yml`'s `build` job runs `make build` on `macos-latest` (arm64) and
  `ubuntu-latest`. The macOS leg is a native build with cgo on, the one
  combination where the bindings compile. No job builds darwin/arm64 with
  cgo off.
- `make lint` for another platform is `GOOS=<os> make lint` (FLOW-17). It
  keeps the host's `GOARCH`, so on an amd64 machine `GOOS=darwin` checks
  darwin/amd64, where the stub is compiled. CI's `lint` and `tier1` jobs
  check the host platform only.
- CI runs `make test` on Ubuntu only. `release.yml` runs on a `v*` tag and
  nowhere else. No job runs `goreleaser check` or a snapshot, and
  `goreleaser check` reads the configuration without compiling anything.

What would have caught it: compiling, on every PR that changes code, each
target a release compiles, the way the release compiles it.

## Prior art

- **`vm-driver-release-plan`** (active): the vz driver's roadmap. Its open
  item "Relay VM Image Distribution" says building from source is friction
  for non-dev users; it has no decision that a release carries a vz driver,
  and none on how `warden` is built for macOS. Not changed here.
- **`vz-image-prepare-refresh`** (archived): the last change confined to
  `driver/vz`. Reused: its reading of FLOW-09 for a change in that tree
  (macos-vz, blocking), and `Preflight` as the first thing `image prepare`
  runs, before the root check and any download. This design's message
  comes out of the same call.
- **`ci-path-filters`** (archived): the `build` job, its `if:` on changed
  paths, and `ci-ok`. Reused as it is: the guard is a step of `build`, so
  it runs on the PRs that job runs on and is already under `ci-ok`.
- **`rule-running-line-match`** (archived): which workflow jobs are rule
  enforcement (FLOW-14). `build` runs none of tier 1, `golangci-lint` or
  `tools/workflow`, so a step added to it is not a rule change.
- **GLOBAL-07**: platform-specific logic sits behind build-tagged files
  "with stubs so every target compiles". The fix makes `driver/vz` meet it
  for the one target it misses. **UX-04**: a failure with a clear cause is
  a `driver.FixableError` that names its fix; the vz driver's platform and
  entitlement errors already are.
- **`prebuilt-artifacts`** (in design, on its own branch): found the fault
  (its **Inventory**, fault 3) and plans `goreleaser check` and a snapshot
  release in the `build` job (its Decision 12). **The guard** below says
  how the two sit together.

## Decisions

1. **The stub is compiled wherever the bindings are not.** `vz_stub.go`'s
   constraint becomes `!darwin || !arm64 || !cgo`, and
   `vz_darwin_arm64.go`'s becomes `darwin && arm64 && cgo`, which states
   what `import "C"` already implies. The two are now complements in the
   text, so every build compiles exactly one.
   - *Rejected: change the stub only.* It compiles, but the pair still
     reads as "darwin/arm64 or not", which is how the gap was written.
   - *Rejected: a third file for darwin/arm64 without cgo.* It would
     repeat the stub.

2. **A released macOS arm64 `warden` stays a build without cgo. It has no
   vz driver, and says so.** This is what the documentation has said since
   the driver landed (`a479f9d`, 2026-05-31): "The released binary is ready
   for the container driver out of the box", and vz's first-time setup
   begins "After building from source on macOS Apple Silicon".
   `CONTRIBUTING.md` lists the Xcode Command Line Tools "for cgo and
   Objective-C" under the vz driver. Nothing in the release path has ever
   built with cgo, run on a macOS runner, or signed `warden` with the
   virtualization entitlement, which the driver also needs.
   - *Rejected: build the macOS release with cgo, on a macOS runner, signed
     with the entitlement.* The history does not ask for it. The nearest
     thing is `45b2ca2` (2026-05-31), which put `warden-io-darwin-arm64` in
     every archive "for VZ users", and `03cfc0b`, which added the macOS
     relay: the two files a vz build needs beside a `warden` that can run
     one. They were added to a release whose `warden` could not, and
     `install.sh` installs neither. A release with a working vz driver is a
     feature of its own (a second runner, a signing identity, and how the
     archives are joined), and it would not replace this fix: a build
     without cgo must still compile (GLOBAL-07). It is not filed by this
     item (**Handed on**).

3. **A build with the stub never passes the vz platform check.** Today
   `checkPlatform` asks only for darwin/arm64 and the entitlement. With the
   stub compiled on darwin/arm64 that is no longer enough: a signed `warden`
   without cgo would pass it, and the stub's `createLinuxVM` returns
   `checkPlatform()`, so a build would get a nil VM and no error. The check
   therefore also asks whether the bindings are in this build, before it
   looks at the entitlement, and fails with a fixable error when they are
   not (**The message**).
   - *Rejected: give the stub functions their own error.* The check runs
     first in every entry point (`StartBuild`, `Exec`, `Preflight`), before
     a relay starts or an image is resolved (UX-04, "check as early as it
     is cheap"). The stub functions are reached only after it.

4. **The guard compiles what a release compiles, in the `build` job.** A
   `make release-build` target and one step that runs it on the job's
   Linux leg (**The guard**).
   - *Rejected: run goreleaser (`goreleaser build --snapshot`).* It reads
     the targets from the configuration, which a list in the `Makefile`
     does not. But it puts goreleaser on every PR and on every machine
     that runs the check, its `before` hook cannot run today, and
     `prebuilt-artifacts` already designs that step with the hook
     rewritten. Two items would write the same lines.
   - *Rejected: a Go test that runs the cross-builds.* Four cold
     cross-builds take over a minute; `make test` is the fast gate
     (TEST-01).
   - *Rejected: a new CI job.* The loop profile lists the Build checks as
     the `build`, `tidy` and `book` jobs (PROFILE-01); a fourth job would
     change that entry, which is a rule change this fix does not need.

5. **Native validation on macos-vz blocks.** FLOW-09: the change is in
   `driver/vz` (**Validation plan**).
   - *Rejected: tracked.* The argument for it is that a build with cgo
     compiles the same files after the change as before (shown below with
     `go list`), and CI's macOS leg compiles them. But FLOW-09 decides by
     where the change is, and a wrong constraint here would leave the
     driver out of the build people run vz with, which only that host
     shows.

## Questions

### Blocking

None.

### Assumed

None. Decision 2 is not an assumption this design rests on: under either
answer to "should a release carry a working vz driver", the build without
cgo has to compile and to say what it lacks, and this item changes nothing
else.

## Design

The item works only in this repository.

### The fix

| File | Change |
|---|---|
| `driver/vz/vz_stub.go` | `//go:build !darwin \|\| !arm64 \|\| !cgo`; a constant that says the bindings are not in this build |
| `driver/vz/vz_darwin_arm64.go` | `//go:build darwin && arm64 && cgo`; the same constant, true |
| `driver/vz/driver.go` | `checkPlatform` takes its verdict on the platform and the bindings from one function of `(GOOS, GOARCH, bindings)`, then checks the entitlement as today |

The function is pure, so a test on any host covers every row:

| GOOS/GOARCH | Bindings | Result |
|---|---|---|
| not darwin/arm64 | no | today's error: "vz driver requires macOS on Apple Silicon (got …)" |
| darwin/arm64 | no | the new error (**The message**) |
| darwin/arm64 | yes | nil; the entitlement check follows, as today |

Tried for this design with a build overlay over `3cb0b2b` (nothing of it is
committed): all six targets of the table below compile `cmd/warden`,
`cmd/warden-io` and `cmd/relay`; `go build ./...` and `go vet ./...` pass
for darwin/arm64 and darwin/amd64; `go test ./driver/vz/` passes. What
`go list` reports for `driver/vz` on darwin/arm64:

| Build | `main` | With the fix |
|---|---|---|
| cgo on | bindings (cgo), `vz_darwin.m`; stub ignored | the same |
| cgo off | neither file: does not compile | stub; bindings and `vz_darwin.m` ignored |

So the build people run vz with compiles the files it compiles today, plus
one constant.

### The message

A `driver.FixableError` (UX-04), printed as every fixable error is:

```
ERROR: this warden was built without cgo, so it has no vz driver (released binaries are built this way)
  To fix: build warden from source on this Mac (xcode-select --install, then make build), or use --driver container or --driver qemu
```

It comes from `checkPlatform`, the first thing the driver does, so on
darwin/arm64 in a build without cgo it is what these give before the driver
starts, downloads or asks for anything:

- `warden build --driver vz` and `warden shell --driver vz`;
- `warden doctor --driver vz`;
- `warden image prepare --driver vz`, before the root check.

They exit 1 (UX-09). Commands that only read the vz image cache (`image
list`, `image verify`, `clean --driver vz`) are pure Go and do what they do
in any build. The container and qemu drivers do not pass through this
check.

### The guard

`Makefile`:

```make
# What a release compiles (.goreleaser.yaml): each program, for each target,
# without cgo. Nothing is kept. A target added there is added here.
RELEASE_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
release-build:
	@for t in $(RELEASE_TARGETS); do \
		echo "== $$t"; \
		GOOS=$${t%/*} GOARCH=$${t#*/} CGO_ENABLED=0 \
			go build ./cmd/warden ./cmd/warden-io ./cmd/relay || exit 1; \
	done
```

`go build` of several packages compiles them and writes nothing.
The three programs are the one goreleaser builds and the two its hook
builds. The hook builds `warden-io` and the relay for three of the four
targets; building them for the fourth costs seconds and keeps one list.
The whole target takes 1m19s on a cold cache (4 cores) and runs on any
machine with Go.

`.github/workflows/main.yml`, last step of the `build` job:

```yaml
      # Each target a release builds compiles the way it builds it (design
      # vz-nocgo-build).
      - if: runner.os == 'Linux'
        run: make release-build
```

The job already runs only when a PR changes code, which `Makefile`,
`.goreleaser.yaml` and every Go file are, and `ci-ok` already needs it.

**What it does not do.** The list is a second copy of
`.goreleaser.yaml`'s targets, and nothing compares the two: a target added
to a release and not here is not guarded. That is accepted until the
snapshot below exists, because it reads the configuration itself.

**How it sits with `prebuilt-artifacts`.** That design adds, in the same
job, a check of each executable's version line after `make build`, and on
the Linux leg `goreleaser check` and a snapshot release. A full snapshot
compiles the same four `warden` builds, so it supersedes this step:

- *This item merges first* (expected: it is a few lines and starts now).
  `prebuilt-artifacts` needs no reduced configuration (its **Releases and
  install** runs the snapshot without the darwin targets only until this
  fix merges). When it merges `main` it meets this step next to the ones
  it adds; the resolution is that its snapshot step takes this step's
  place. It then decides in its consolidation whether `make
  release-build` stays as the check that needs no goreleaser, or goes
  (FLOW-10, FLOW-15).
- *`prebuilt-artifacts` merges first.* This item's Publish removes that
  design's reduction, as it asks, and adds no step of its own: the
  snapshot is the guard. `make release-build` is then kept only if that
  design kept a use for it.

Both items touch the end of one job, so the second to merge resolves one
hunk. They change no other line in common.

### The same fault elsewhere

Cross-builds on `main` at `3cb0b2b`, `CGO_ENABLED=0`, go1.26.3 on
linux/amd64. `R` marks what a release compiles (goreleaser, its hook, or
`release.yml`'s support images).

| Target | `cmd/warden` | `cmd/warden-io` | `cmd/relay` | `cmd/warden-pause` | `cmd/collector` | `go build ./...` | `go vet ./...` |
|---|---|---|---|---|---|---|---|
| linux/amd64 | ok R | ok R | ok R | ok R | ok | ok | ok |
| linux/arm64 | ok R | ok R | ok R | ok R | ok | ok | ok |
| darwin/amd64 | ok R | ok | ok | ok | ok | ok | ok |
| darwin/arm64 | **fails** R | ok R | ok R | ok | ok | **fails** | **fails** |
| windows/amd64 | ok | ok | ok | ok | ok | ok | ok |
| windows/arm64 | ok | ok | ok | ok | ok | ok | ok |

The three failures are one: `driver/vz`, and `cmd/warden` because it
imports it. With the fix tried as above, the darwin/arm64 row is `ok`
throughout, so nothing else was hidden behind it. `cmd/warden-vmnet` holds
one C file and no Go, and nothing builds it. `tools/workflow` and
`tools/tier1` build for darwin/arm64 and windows/arm64. The only other
file in the repository that imports `"C"` is a tier-1 analyzer's test
fixture.

### Reference docs (UX-01) and the overview (FLOW-15)

- `docs/book/src/guide/drivers.md`, VZ **Requirements**: a `warden` built
  from source with cgo (`make build`); a released binary has no vz driver
  and says so.
- `docs/book/src/guide/cli.md`, `warden doctor`: the vz check also covers a
  `warden` built without cgo.
- `docs/book/src/getting-started/installation.md`: the sentence on what
  the released binary is ready for names the vz driver as needing a build
  from source, and points at the first-time setup.
- `docs/book/src/contributing/structure.md`: `vz_stub.go` is the stub for
  every build without the bindings.
- `CONTRIBUTING.md`: `make release-build` in the command list and in
  "Cutting a release".
- `overview.md`: no component's flow changes.

### Handed on

Findings that are not this item's. This agent edits only its own queue
item, so the session that receives this design files or passes them on.

| What | Where it goes |
|---|---|
| The guard's step and the snapshot step meet at the end of the `build` job; which one stays, and whether `make release-build` does | `prebuilt-artifacts`, as **The guard** sets out: a note on its queue item |
| `prebuilt-artifacts`' **Releases and install** says "vz has everything once a macOS arm64 archive can be built". The archive will hold the files a vz build needs, but its `warden` is built without cgo and has no vz driver | the same note: that design's wording, and its error table if it lists what a release cannot run |
| A release whose macOS arm64 `warden` runs the vz driver (cgo on a macOS runner, the entitlement, a signing identity) | Jeff's to want or not. A new feature, so intake files it on hold under the freeze (FLOW-20) |
| `make integration-test-vz` runs `TestRelayVMBoot` under a build tag `integration`; `driver/vz` has neither any more | a small cleanup item, or `driver-contract`, which reworks the vz build |
| No CI job compiles `cmd/warden` for Windows (the Windows legs compile `tools/tier1` and `cmd/warden-io` only); it compiles today, and `GOOS=windows make lint` is what checks it (FLOW-17) | GLOBAL-07's automated part; not a release target, so not this guard's |

## Defaults and overrides

None. The change adds no flag, config key or default, and no merge or
fallback. Which platform file a build compiles is decided by the build's
own settings, and a build without the bindings does not fall back to
another driver: `--driver vz` is a provided value that this build cannot
use, so it stops with an error that names the value's fix (GLOBAL-10,
GLOBAL-11, UX-04).

## Cross-driver impact

- **vz**: a build for darwin/arm64 without cgo compiles and reports that it
  has no vz driver. A build with cgo compiles the files it compiles today;
  its platform check gains a branch that is false at compile time. Builds
  for other platforms give the error they give today.
- **container, qemu, hyperv, firecracker**: not reached. The change is in
  `driver/vz`'s own files; nothing under the shared `driver/` packages,
  `relay/`, `ledger/` or `collector/` changes (ORCH-01). They gain one
  thing: on an Apple Silicon Mac a released `warden` exists again to run
  them.

## Validation plan

**Tier 0** (TEST-01, GLOBAL-04), in `make test` on every platform:

- a table test of the platform function over the rows of **The fix**: each
  error is a `driver.FixableError` with a fix, the darwin/arm64 row without
  bindings carries the message above, and no row without bindings returns
  nil (Decision 3);
- a test that the constant agrees with the build it runs in: true only on
  darwin/arm64 with cgo. On Linux it shows false; on the Mac, under `make
  test`, it shows the bindings are compiled.

**The guard**, in CI on every PR that changes code: `make release-build`.
It fails on `main` today and passes with the fix.

**FLOW-17**: `make test`, `make lint`, `GOOS=darwin make lint`, and
`GOOS=darwin GOARCH=arm64 make lint`, which is the combination the plain
form misses on an amd64 machine.

**Tier 1**: not reached. No container-driver or shared package changes.

**Tier 2, native, blocking: macos-vz** (FLOW-09). The change is in
`driver/vz`, and what it decides, which file a build on that host
compiles, shows only there. Queue step `f6-validate-macos-vz`, on the
branch after consolidation:

1. `make build` (cgo on). `go list -f '{{.CgoFiles}} {{.IgnoredGoFiles}}'
   ./driver/vz/` lists `vz_darwin_arm64.go` as cgo and `vz_stub.go` as
   ignored. `make test` passes. `warden doctor --driver vz` prints what
   `main`'s prints. A vz build succeeds and its ledger verifies.
2. `CGO_ENABLED=0 go build -o <scratch>/warden ./cmd/warden` compiles; on
   `main` it fails with the errors of **Problem** (capture that first).
   With that binary, `doctor --driver vz`, `build --driver vz`, `shell
   --driver vz` and `image prepare --driver vz` (without sudo) each print
   the ERROR and To fix lines of **The message**, exit 1, and start no
   download, VM or relay. `image list` prints what the cgo build's does,
   and `doctor` for the container driver works.
3. The same binary signed with the entitlement (`codesign --sign -
   --entitlements entitlements.plist`) gives the same message, not the
   entitlement's and not a VM error (Decision 3).
4. `make release-build` passes on the Mac.

**Deferred**: nothing. `tier2_gaps` is empty.

## Implementation plan

1. `driver/vz`: the two build constraints, the constant in both files, the
   platform function and its use in `checkPlatform`, with doc comments
   (GLOBAL-07, UX-04, GLOBAL-01).
2. `driver/vz` tests: the table test and the constant's test (GLOBAL-04,
   TEST-01).
3. `Makefile`: `release-build`, in `.PHONY` (GLOBAL-07).
4. `.github/workflows/main.yml`: the step at the end of the `build` job
   (GLOBAL-07, FLOW-14: `build` is not a rule-running job).
5. Reference docs: `drivers.md`, `cli.md`, `installation.md`,
   `structure.md`, `CONTRIBUTING.md` (UX-01).
6. `make test`, `make lint`, `GOOS=darwin make lint`, `GOOS=darwin
   GOARCH=arm64 make lint`, `make release-build` (FLOW-17).

## Design review

## Consolidation

## Rollback notes

## Archive summary
