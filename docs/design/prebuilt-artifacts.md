---
feature: prebuilt-artifacts
status: draft
drivers: [container, qemu, vz, hyperv, firecracker]  # every driver stages an executor or a relay
                         # through driver.GuestBinary, relayvm.Find or goBuildLinux, and each of
                         # those calls changes (FLOW-02, conservatively: driver/ is shared)
seams: [bootstrap-model] # BOOT-02's sentence on the executor's version (BOOT-07). No relay or
                         # executor protocol changes: relaycfg/, relay/ and cmd/warden-io are not
                         # edited beyond the version each binary states
surfaces: [config, output]  # config: what [runtime] relay_image = "dev" and WARDEN_IMAGE=dev
                         # mean. output: <output>/artifacts.json, an Artifacts block in warden
                         # doctor, one line when a file on PATH is passed over, new errors
security_boundary: host-env  # which files the host runs as the relay (vz runs it on the host)
                         # and delivers as the executor, the one input outside the relay. It
                         # also reaches env-relay: the relay binary, image and boot assets
alters_relationships: true  # BOOT-02 is amended (the bootstrap model, FLOW-02), and the
                         # orchestrator stops producing the executor and the relay it runs:
                         # development tooling and releases produce them, warden selects them
design_review: required  # FLOW-19: a security boundary, a seam, a rule change, blocking native
                         # validation and assumed questions
native_validation:
  - {host: macos-finch, blocking: true, reason: "driver/container is reached: a development warden no longer builds the relay, holder and sidecar images or the executor per build; it uses the images make images built in finch's store, which CI (docker on Linux) does not exercise"}
  - {host: macos-vz, blocking: true, reason: "driver/vz is reached: the relay it runs on the host and the executor image prepare installs come from the lookup, under sudo's PATH for prepare; only this host runs them"}
  - {host: windows-hyperv, blocking: true, reason: "driver/hyperv is reached: the Windows executor on the seed comes from the lookup, and the lookup's PATH handling on Windows (';', no current directory) runs with a real build only here"}
  - {host: qemu-any, blocking: true, reason: "driver/qemu is reached: the relay binary, the relay and direct-boot assets come from the lookup, and direct boot starts delivering the executor per build; CI runs no qemu build"}
  - {host: container-any, blocking: false, reason: "the container driver on a Windows host runs the same code CI runs on Linux; what only this host adds is make images under Git Bash and go test ./driver ./cmd/warden on Windows, which CI does not run"}
  - {host: macos-qemu, blocking: false, reason: "a qemu Windows guest takes warden-io-windows-arm64.exe from the lookup; qemu Windows builds do not complete on main (backlog qemu-windows-guest-package)"}
tier2_gaps:
  - {host: container-any, gap: "on the Windows desktop: go test ./driver/... ./cmd/warden; make build and make images CTR=<runtime> under Git Bash; a default container build with the development images; a copy of the executor in a PATH directory is used and one in the working directory is not (backlog prebuilt-artifacts-windows)"}
  - {host: macos-qemu, gap: "a qemu Windows guest build (--guest-os windows) stages dist/warden-io-windows-arm64.exe found by the lookup, and is refused before any VM boots when the file is missing (backlog prebuilt-artifacts-qemu-windows)"}
rules: [GLOBAL-01, GLOBAL-03, GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-09, GLOBAL-10, GLOBAL-11,
        ORCH-01, BOOT-01, BOOT-02, BOOT-06, BOOT-07, RELAY-08, UX-01, UX-02, UX-03, UX-04,
        UX-08, TEST-01, TEST-02, TEST-03, TEST-04, TEST-06, TEST-08, SEC-03, FLOW-01, FLOW-02,
        FLOW-05, FLOW-06, FLOW-07, FLOW-09, FLOW-10, FLOW-12, FLOW-14, FLOW-15, FLOW-16,
        FLOW-17, FLOW-19, FLOW-20, LOOP-02, LOOP-04, LOOP-09]
rule_changes:
  - {rule: BOOT-02, change: amend, paths: [docs/development/bootstrap.md]}
concepts: [prebuilt-artifacts, artifact-lookup, artifact-version, minimum-version, guest-binary,
           executor-delivery, relay-binary, relay-vm-assets, boot-assets, relay-image,
           dev-build, dev-images, module-root, source-tree, release-archive, install-script,
           transitional-detection, security-fix]
related: [guest-binary-cache-per-tree.md, unified-bootstrap.md, build-link-subnet.md,
          vm-driver-release-plan.md, firecracker-driver.md]
---

# `warden` compiles nothing: it finds its artifacts on `PATH`, then beside itself

## Problem

`warden` builds part of what it runs. `driver.ModuleRoot`
(`driver/hostutil.go`) walks up from the executable, and then from the
working directory, to a directory whose `go.mod` declares this module, and
`warden` runs `go build` there: `driver.GuestBinary` for the relay and the
executor of every VM driver, the container driver's `goBuildLinux` for the
relay image, the executor and the holder, and it takes the relay VM's
kernel and initramfs from that tree's `tools/` output (`relayvm.Find`).

That is finding F9 of `runtime-cli-project-path`'s design, restated in
`project-config-trust`'s (**F9: the source tree**, read at `d3efc4a`): a
repository that declares the module supplies source that is compiled on
the host into the relay or the executor, and with the default
`./warden.toml` it can ask for the path that does so (`driver = "qemu"`,
`relay_image = "dev"`). `project-config-trust` makes the small fix, the
tree is found from the executable only, and merges first. A `warden`
inside a tree still compiles after it.

Jeff, 2026-11-15, in `project-config-trust`'s design review (its a6). These
two sentences are the requirement:

> "So warden shouldn't need its own source directory for anything after
> componentization.  It should instead fully be based on pre-compiled
> existing binaries/artifacts which are managed and installed and it may
> have development tooling for helping with that, but it does not manage
> any compilation.  Those are purely development processes"

> "Remove compilation now.  warden looks for necessary artifacts  to be on
> PATH and searches its own directory if they're not found or not on or
> above a minimum version"

This item is that: `warden` looks for no source tree, runs no compiler and
builds no image. It is a security fix, so FLOW-20's freeze does not hold
it. It is not a program item, and the umbrella design is cited below and
not listed in `related`.

## Prior art

- **`guest-binary-cache-per-tree`** (archived). It keyed `GuestBinary`'s
  build cache by source tree so that two checkouts stop staging each
  other's builds. Its Decision 2 says "a release ships its guest binaries
  next to `warden` (lookup 1), so it never relied on the cache". That is
  not what a release does: the archive names the Linux binaries
  `warden-io` and `warden-relay`, and the lookup asks for
  `warden-io-linux-<arch>` (**Inventory**). This item removes the cache
  and the build with it; `warden clean --cache` already removes what the
  cache left.
- **`build-link-subnet`** and **`hyperv-relay-per-build-channel`**
  (archived). They gave each launcher-carrying boot asset a stamp file
  beside it (`<file>.link-channel`, `WARDEN_LINK_CHANNEL=<n>`), written
  by the tool that builds the asset and compared by `warden` before a
  build (`driver.LauncherStamp`). This item reuses that stamp as the
  check a boot-asset set must pass, and keeps its values.
- **`vm-driver-release-plan`** (archived), "Relay VM Image
  Distribution": it listed prebuilt kernel and initramfs in releases,
  embedding them in `warden`, or a download on first use, and
  recommended embedding. None was built. This item does not choose among
  them (a5).
- **`unified-bootstrap`** (archived): BOOT-02 as it stands, and the
  per-driver delivery this item leaves alone except on qemu's direct boot
  (Decision 8).
- **`firecracker-driver`** (archived): `relayvm.AppendFileCPIO`, which
  appends the executor per build to a launcher-only initramfs. Decision 8
  reuses it for qemu.
- **`componentization`** (archived, the umbrella). Where the pieces end
  up: the relay is released from its own repository in forms with
  published hashes (seam 8, track M); the executor stays with the kit;
  each driver becomes its own binary (K1, K2) and repository (K3, K4).
  **How it sits with the other items** says what each inherits.
- **`driver-contract`** (K1, in review; read at `a6c8f0b`), **Discovery
  and trust** and its Security note. Jeff's answer to its a16 is the model
  for this lookup: `PATH` wins over a bundled copy, then `warden`'s own
  directory; a file reached through the working directory or a relative
  `PATH` entry is not used; the only guard is that no config file names
  the file by path; heavier guarding is "overcontrolled ... this is never
  outside of the user's control (i.e. never a part of the build
  environment)". **The lookup** says where this one differs and why.
- **`project-config-trust`** (in Plan; read at `d3efc4a`) and
  **`runtime-cli-project-path`** (approved at `a2afd08`; branch read at
  `8e41bd8`): one `warden.toml` per build; a setting is validated by its
  consumer at initialization, after the overrides, only for what the run
  uses (UX-08 as that item amends it); a change that refuses what worked
  before is an error at once with a concrete message, and the code that
  detects the old state is kept until 1.0 under a `TRANSITIONAL(<name>)`
  marker.
- A design named `baked-asset-staleness` is referred to by two backlog
  entries; it is not on `main` and nothing here relies on it.

## Decisions

Terms. An **artifact** is a file, or a set of files that go together,
that `warden` needs for a build and does not contain: an executor, a
relay binary, a boot-asset set. A **candidate** is a directory that holds
every file of the artifact. **Beside `warden`** is the directory of
`warden`'s executable, symbolic links resolved.

1. **`warden` runs no compiler, builds no image and reads no source
   tree.** `driver.ModuleRoot`, `driver.GuestBinary`'s cache and build,
   `goBuildLinux`, `buildRelayFromSource`, the build of the holder and
   sidecar images from embedded Containerfiles, and `relayvm.Find`'s tree
   lookup are deleted. Nothing takes their place at run time.
   - *Rejected: keep the build for a `warden` inside its tree*, which is
     what `project-config-trust` leaves. Jeff: "it does not manage any
     compilation".
   - *Rejected: keep the image builds, since they read no tree.* They
     are builds `warden` manages, and the holder's needs the tree.
   - *Dropped fallbacks* (GLOBAL-11): "not beside `warden`, so build it"
     (`GuestBinary`, steps 2 and 3); "not beside `warden`, so look in the
     tree" (`relayvm.Find`); "no `warden-io` beside a release, so build
     it" (`buildWardenIO`).

2. **One lookup for every artifact: the first candidate on `PATH`, then
   beside `warden`** (**The lookup**). It is K1's driver lookup with
   three differences that Jeff's sentence or the nature of the files
   forces: a candidate must pass a check, and one that does not is passed
   over for the copy beside `warden`; a candidate need not be executable,
   because the host runs almost none of these files; and nothing names
   an artifact by path.
   - *Rejected: every acceptable candidate on `PATH` in turn.* His words
     name two places. A second copy later on `PATH` is passed over, as
     K1's is.
   - *Rejected: a flag, variable or key that names an artifact's file.*
     K1's one guard is that no config file names a driver's file; here no
     source does, so there is nothing to guard. `PATH` is the override.
   - *Rejected: executables on `PATH`, other files only beside
     `warden`.* Two rules where his sentence has one (a1).

3. **The minimum is `warden`'s own version; a newer artifact is
   accepted.** An executor or relay binary is accepted when the version
   it states is at or above the version of the `warden` that looks for
   it. A development `warden` (version `dev`) accepts only `dev`, and a
   release never accepts `dev` (**The version**). BOOT-02 says the
   executor "matches the orchestrator's version"; "at or above" is not
   that, so BOOT-02 is amended, with a Bootstrap comparison (a2).
   - *Rejected: exactly `warden`'s version.* It keeps BOOT-02 as
     written and refuses what Jeff's "on or above" accepts. It also
     cannot outlive K3, when a driver in its own repository has a version
     that is not the executor's.
   - *Rejected: a minimum per artifact, kept by hand in code*, lower
     than `warden`'s version. It is what a driver in its own repository
     will need (K3). While everything is built from one tree and released
     together it is a table to forget to bump, and it would let a release
     run an older executor from `PATH` in place of the one it shipped
     with.

4. **An executable states its version in itself, as a line of bytes
   `warden` reads without running the file** (**The version**).
   - *Rejected: run it with `--version`.* Most of these files are for
     another OS or architecture and cannot run on the host; and the
     lookup would execute every candidate it considers, a file on `PATH`
     it then rejects included.
   - *Rejected: Go's build information* (`debug/buildinfo`, what `go
     version -m` prints). Tried for this design: a binary built in this
     worktree states `v0.6.1-0.20261115193423-ad4ea5002e24`, read from a
     Windows and a macOS binary on Linux. It needs no new format and
     orders development builds. It was not chosen because the version is
     then whatever the toolchain derives from the checkout: `go run`, a
     source archive and `-buildvcs=false` give `(devel)`, a shallow clone
     gives `v0.0.0-...`, it is a second version beside the one `warden
     --version` prints, and a relay not written in Go cannot state it
     (a3).
   - *Rejected: a sidecar file per artifact, or a manifest per
     directory.* A copied binary loses its sidecar; a manifest in a
     `PATH` directory is shared by whatever else is installed there.

5. **A boot-asset set is checked by its stamp, not by a version.** A
   kernel and an initramfs do not change with `warden`'s release (the
   relay is appended per build), and requiring `warden`'s version would
   mean rebuilding a kernel for every release. The check they already
   have, the link-channel stamp, is the one the lookup applies; a set
   with no stamp defined (the relay VM's kernel and base initramfs) is
   accepted when complete (a5).

6. **Container images are named by reference, not found by the lookup.**
   A release pulls what it pulls today. A development `warden` uses three
   local images that `make images` builds: `warden-relay:dev`,
   `warden-holder:dev`, `warden-netns:dev`. `relay_image = "dev"` keeps
   working and means "the local image `warden-relay:dev`" (a4).
   - *Rejected: remove the value `dev`.* It is a documented value
     (UX-03) and still has a meaning.
   - *Rejected: copy the relay binary into a container made from the
     released image*, as the executor is copied into the build container.
     It would make the container driver's relay a file like every other
     driver's. It changes how a release's relay container is made, which
     is more than removing compilation, and the relay's end state is an
     OCI image released by the relay repository (seam 8).
   - *Rejected: a version label on images, checked against the
     minimum.* A provided reference is used as given (GLOBAL-10), and the
     release's references are compiled in. It would catch only a stale
     development image, which `make images` cures.

7. **Development tooling places everything beside `dist/warden`**
   (**Development tooling**). `make build` builds every executable the
   host's drivers need and copies the boot assets the tree has built into
   `dist/`; `make images` builds the three development images.

8. **qemu's direct boot delivers the executor per build.** Found while
   taking the inventory: `tools/build-vm/build-initramfs.sh` compiles
   `warden-io` into the direct-boot initramfs, and the qemu driver boots
   that initramfs as it is, so the executor of a qemu direct-boot build is
   the one from the day the asset was built. That is not "delivered per
   build" (BOOT-02), it is not in the deviation register, and it would be
   the one executor the lookup never sees. The qemu driver appends the
   executor the lookup found, as Firecracker does
   (`relayvm.AppendFileCPIO`, at `bin/warden-io`, where a later archive
   replaces the baked file), and the script stops compiling it. This
   brings a driver into line with the model and is not a model change
   (BOOT-07) (a6).
   - *Rejected: hand it to a follow-up.* The Security note would then
     have to except one path from "the executor is the file the lookup
     found".

9. **A release ships each artifact under the name it is looked up by,
   and `install.sh` installs every file of the archive** (**Releases and
   install**). The names `warden-io` and `warden-relay` go. An old
   `warden-io` beside a new `warden` is detected and named in the error
   until 1.0 (`TRANSITIONAL(artifact-names)`) (a7).
   - *Rejected: accept `warden-io` as a second name.* The file an older
     install left states no version and would be refused anyway.

10. **What was used is recorded.** Every build writes
    `<output>/artifacts.json`; `warden doctor --driver <name>` lists the
    driver's artifacts; a file on `PATH` that is passed over is said on
    stderr on every run. No command is added.

11. **This item does not ship boot assets in a release, and adds no
    Windows release.** Neither exists today. The error for a missing
    boot-asset set says that no release ships it and how to build and
    place it (a5).

## Questions

### Blocking

None.

### Assumed

Most consequential first. Each is confirmed in the design review
(FLOW-19).

**a1. The lookup, made precise.** Jeff's sentence gives the two places
and the trigger. Proceeding with: the *first* candidate on `PATH`, and no
later one; a candidate reached through a relative or empty `PATH` entry
stops the lookup with an error, as K1's does; then beside `warden`; no
flag, variable or key names an artifact's file; the same lookup for a
kernel and an initramfs as for an executable; a newer artifact is used
without comment; `artifacts.json`, the doctor block and the passed-over
line show what was used. Confirm.

- (A) as above. *Recommended*: it is K1's lookup plus exactly what the
  sentence adds.
- (B) every `PATH` entry in turn until one is acceptable, then beside
  `warden`. More forgiving, and which file wins is then harder to
  predict.
- (C) executables by this lookup; kernels and initramfs only beside
  `warden`. `PATH` then holds only programs, at the cost of a second
  rule.

**a2. The minimum, and BOOT-02.** Proceeding with: the minimum is the
version of the `warden` that looks; newer is accepted; `dev` takes only
`dev`; BOOT-02 is amended from "matches the orchestrator's version" to
"at or above the orchestrator's" (**Rule changes**, **Bootstrap
comparison**), which makes this item hold one of FLOW-05's
`alters_relationships` slots (two of three are held). Confirm.

- (A) as above. *Recommended*: "on or above a minimum version" in his
  words, and the form that survives K3.
- (B) exactly `warden`'s version; a newer file on `PATH` is passed over
  like an older one. BOOT-02 stands as written, no rule change, no
  Bootstrap comparison, `alters_relationships: false`. A customized
  executor must then be built with the release's own version.
- (C) a minimum per artifact, kept in code and lower than `warden`'s
  version. BOOT-02 amended as in (A).

**a3. How an executable states its version.** Proceeding with: one line
of bytes in the file, `buildwarden-version=<version>;`, set when the
binary is linked (by `make build`, to `dev` unless told otherwise, and by
goreleaser) and found by reading the file. It is the version `warden
--version` prints. Confirm.

- (A) as above. *Recommended*: one version, said by whoever builds the
  binary, the same in any language and under any way of building. Cost: a
  format of this project's own, and development builds are all `dev`, so
  a stale file in `dist/` is not detected (it was rebuilt automatically
  before; `make build` rebuilds everything).
- (B) Go's build information, read with `debug/buildinfo`. Nothing to
  invent, `go version -m` shows it, development builds are ordered by
  commit, and the file's OS and architecture can be checked too. Cost:
  no version without a git checkout with tags (**Decisions**, 4), two
  versions per binary, Go only.
- (C) a sidecar file beside each artifact, or one manifest per
  directory.

**a4. The container driver's images.** Proceeding with: a development
`warden` uses `warden-relay:dev`, `warden-holder:dev` and
`warden-netns:dev` from the runtime's local images, built by `make images
CTR=<runtime>`, never pulled and never built by `warden`; `relay_image =
"dev"` and `WARDEN_IMAGE=dev` mean the local `warden-relay:dev` for any
`warden`; a release's defaults are untouched (the relay from
`ghcr.io/buildwarden/relay:latest`, the holder and sidecar by digest); no
version is checked on an image. Confirm.

- (A) as above. *Recommended.*
- (B) as (A), and the value `dev` is removed: a release refuses it with
  the fix, and a development `warden` needs no key to use its images.
- (C) as (A), and each development image carries a version label that
  `warden` checks like an executable's.

The release default being the moving tag `:latest` is as on `main`, and
K1's design notes it; pinning it belongs with seam 8's published hashes
and is not proposed here.

**a5. Boot assets and what a release ships.** Proceeding with: boot-asset
sets (qemu's relay VM and direct-boot kernel and initramfs, Firecracker's
three files) are found by the same lookup, each set from one directory,
and checked by the link-channel stamp where the set has one; no release
ships them, as today, and the error says how to build and place them;
Hyper-V's relay boot VHDX stays where it is (the hyperv cache, or
`WARDEN_RELAY_BOOT_VHDX`), since `warden` neither compiles it nor looks in
a tree for it; no Windows release is added. Confirm.

- (A) as above. *Recommended*: the item removes compilation and changes
  no release's reach. The relay's appliance is track M's to release (seam
  8), and a driver's own boot assets are its repository's (K3, K4).
- (B) the release workflow also builds the qemu and Firecracker boot
  assets and ships them in the archive, so an installed release runs
  those drivers with nothing built. It adds a kernel build and Alpine
  downloads to every release, and a new thing to validate natively.
- (C) as (A), and every boot-asset set also gets a version file and a
  minimum in code, so that one rule covers every artifact.

**a6. qemu's direct boot (Decision 8).** Proceeding with: fixed here, by
appending the executor per build. Confirm.

- (A) fixed here. *Recommended*: a dozen lines in a driver this item
  already changes and validates on `qemu-any`.
- (B) filed as its own item; this item's Security note then says the
  direct-boot executor is the asset's.

**a7. The release's file names (Decision 9).** Proceeding with: the
archive carries `warden-io-<os>-<arch>` and `warden-relay-<os>-<arch>`
and no longer `warden-io` and `warden-relay`; `install.sh` installs every
artifact in the archive and leaves an older `warden-io` where it is; the
error for a missing executor names an older install's `warden-io` when
one is beside `warden`, until 1.0. Confirm.

- (A) as above. *Recommended*: one name per file, the same in `dist/`,
  in the archive and on `PATH`.
- (B) the archive carries both names for the Linux binaries until 1.0.
  Nothing reads the old names, so it only delays the day a hand-made
  upgrade script fails.

**a8. How this item waits for `project-config-trust`.** Its merge comes
after that item's (Jeff, that item's a6). Both edit `driver/hostutil.go`,
two drivers' test files and the same doc sentences, and this item deletes
what that one adds. Proceeding with: Plan starts after that item's PR has
merged, held as K1 is: a blocking question naming the merge, with a
`human` step that `f2-plan` needs, written by the session that records
this design's review. Confirm.

- (A) as above. *Recommended*: Implement then deletes code that is on
  `main`, and it costs nothing if that item merges first anyway.
- (B) no hold: Plan and Implement run now on `main` at `ad4ea50`, and
  Publish's merge of `main` resolves the overlap. Sooner if that item is
  slow; the conflicts are deletions.

## Design

This item works only in this repository (LOOP-04). It adds or moves no
relay setting (RELAY-08): what it chooses is which relay file is started,
which the rule calls "how the relay is started, not configuration for a
build".

### Inventory

Every place where `warden`, a driver or a tool `warden` invokes compiles
or builds something at run time, or looks into a source tree, on `main`
at `ad4ea50`. Found by reading every `exec.Command` under `cmd/` and
`driver/`, every `ctrctl.ImageBuild`, and every use of `ModuleRoot`.

| # | Where | What it does | Artifact | Driver, platform |
|---|---|---|---|---|
| 1 | `driver.ModuleRoot` (`driver/hostutil.go`) | Finds the tree: up from the executable, then from the working directory | the tree itself | all |
| 2 | `driver.GuestBinary` (`driver/hostutil.go`) | Beside `warden` under `warden-io-<os>-<arch>[.exe]` or `warden-relay-<os>-<arch>`; else a per-tree cache; else `go build ./cmd/<pkg>` into the cache | see rows 2a to 2g | |
| 2a | `relayvm.RelayInitrd` (`driver/relayvm/initrd.go`) | relay, appended to the base initramfs per build | `warden-relay-linux-<arch>` | qemu (host arch), firecracker (amd64) |
| 2b | `linuxguest.Seed` (`driver/linuxguest/seed.go`) | executor on the cloud-init seed | `warden-io-linux-<arch>` | qemu Linux guest, firecracker disk path |
| 2c | `directBoot` (`driver/firecracker/deliver_linux.go`) | executor appended to the launcher initramfs | `warden-io-linux-amd64` | firecracker direct boot |
| 2d | `windowsSeedDir` (`driver/qemu/windows.go`) | executor on the `WARDEN` seed | `warden-io-windows-<arch>.exe` | qemu Windows guest |
| 2e | `stageSeedDir` (`driver/hyperv/winseed.go`) | the same | `warden-io-windows-<arch>.exe` | hyperv |
| 2f | `startHostRelay` (`driver/vz/driver.go`) | relay copied, signed and run on the host | `warden-relay-darwin-arm64` | vz |
| 2g | `installWardenIO` (`driver/vz/image.go`) | executor baked at `image prepare` (deviation D2) | `warden-io-darwin-arm64` | vz |
| 3 | `goBuildLinux` (`driver/container/images.go`) | `go build` for Linux in the tree | rows 3a to 3c | container |
| 3a | `buildRelayFromSource` (`container.go`) | builds `./cmd/relay`, then an image `warden-relay:<build id>` from an inline Containerfile on `alpine:latest`; removed after the build | relay image | container: version `dev`, or `relay_image = "dev"` on any version |
| 3b | `buildWardenIO` (`container.go`) | a release reads `warden-io` beside `warden`; a `dev` version, or a release that does not find the file, builds `./cmd/warden-io` | executor | container |
| 3c | `buildHolderImage` (`images.go`) | builds `./cmd/warden-pause` and an image tagged by content hash | holder image | container, when no digest is compiled in (a development build) |
| 4 | `buildNetnsImage` (`images.go`) | builds the sidecar image from an embedded Containerfile; no tree, but an image build | sidecar image | container, development build |
| 5 | `relayvm.Find` (`driver/relayvm/assets.go`) | All files beside `warden` under release names, else all from `tools/*/output/` in the tree | `relay-vm-vmlinuz`, `relay-vm-initramfs.cpio.gz`; `build-vm-vmlinuz`, `build-vm-initramfs.cpio.gz`; `firecracker-vmlinux`, `firecracker-relay-initramfs.cpio.gz`, `firecracker-build-initramfs.cpio.gz` | qemu, firecracker |
| 6 | `relayvm.StampPath` | The stamp is `link-channel` in the tree's output directory, or `<file>.link-channel` beside `warden` | the stamps of row 5 | qemu, firecracker |

Not compilation by `warden`, and left as they are:

- The tools under `tools/` are run by a developer, CI or a native
  session, never by `warden`; `warden` only names them in fix messages.
  `tools/relay-vm/build-initramfs.sh`, `tools/firecracker-vm/build-*.sh`
  and `tools/build-vm/build-initramfs.sh` build the boot assets (the last
  one compiles `warden-io` into the initramfs: Decision 8).
  `tools/relay-vm/hyperv/refresh-cache.sh` compiles the relay into the
  Hyper-V relay boot VHDX and installs it in the hyperv cache.
  `tools/vz-image-prep` is a manual tool with its own tree lookup.
- Hyper-V's relay boot VHDX is found in the hyperv cache or through
  `WARDEN_RELAY_BOOT_VHDX` (`driver/hyperv/buildassets.go`), with its own
  stamp. No tree, no compiler.
- Downloads (`curl` for cloud images and IPSWs), `codesign`, `qemu-img`
  and the container runtime's `pull` build nothing.

What a release ships today (`.goreleaser.yaml`, `install.sh`,
`release.yml`, `relay-image.yml`), and what a developer has:

| Artifact | In the release archive | Installed by `install.sh` | Looked up as | `make build` |
|---|---|---|---|---|
| executor, Linux, host arch | `warden-io` | yes | `warden-io` (container, release); `warden-io-linux-<arch>` (`GuestBinary`) | `dist/warden-io-linux-{amd64,arm64}` |
| relay, Linux, host arch | `warden-relay` | no | `warden-relay-linux-<arch>` | `dist/warden-relay-linux-{amd64,arm64}` |
| executor and relay, macOS arm64 | `warden-io-darwin-arm64`, `warden-relay-darwin-arm64`, in every archive | no | those names | on macOS arm64 only |
| executor, Windows | not shipped | | `warden-io-windows-<arch>.exe` | not built; `GuestBinary` builds it on first use |
| boot assets (row 5) | not shipped | | the release names of row 5 | not built; the scripts write `tools/*/output/` |
| relay image | `ghcr.io/buildwarden/relay:latest` and `:v<version>` (`relay-image.yml`) | pulled | `:latest` | built per build |
| holder, sidecar images | by digest, compiled into `warden` (`release.yml`) | pulled | the digest | built per build, tagged by content hash |
| Hyper-V relay boot VHDX | not shipped | | the hyperv cache | `refresh-cache.sh` |

So today an installed release runs the container driver and nothing else
without a source tree: the Linux binaries are in the archive under names
no VM driver looks for, and no archive has boot assets. There is no
Windows release. And `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build
./cmd/warden`, which is goreleaser's configuration for the macOS arm64
archive, fails at `ad4ea50` (run for this design: `driver/vz/vm.go:
undefined: linuxVMConfig`, because the cgo file and its stub are both
excluded). That is not this item's to fix (**Handed on**); the macOS rows
below say what the archive carries once it builds.

### The artifacts and their names

The closed list. A driver asks for an artifact by one of these names and
no other.

| Kind | File name | Used by |
|---|---|---|
| executor | `warden-io-linux-amd64`, `warden-io-linux-arm64` | container (host arch), qemu Linux guest (host arch), firecracker (amd64) |
| executor | `warden-io-windows-amd64.exe`, `warden-io-windows-arm64.exe` | hyperv, qemu Windows guest |
| executor | `warden-io-darwin-arm64` | vz, at `image prepare` |
| relay | `warden-relay-linux-amd64`, `warden-relay-linux-arm64` | qemu, firecracker |
| relay | `warden-relay-darwin-arm64` | vz |
| boot assets: qemu relay VM | `relay-vm-vmlinuz` + `relay-vm-initramfs.cpio.gz` | qemu |
| boot assets: qemu direct boot | `build-vm-vmlinuz` + `build-vm-initramfs.cpio.gz` + `build-vm-initramfs.cpio.gz.link-channel` | qemu, builds with no disk image |
| boot assets: firecracker | `firecracker-vmlinux` + `firecracker-relay-initramfs.cpio.gz` + `firecracker-build-initramfs.cpio.gz` + `firecracker-build-initramfs.cpio.gz.link-channel` | firecracker |

These are the names `GuestBinary` and `relayvm` use today for a file
beside `warden`; none is new. K1's convention `warden-driver-<name>` names
a program `warden` runs with commands. An artifact is a file a driver
delivers into a guest, boots or (the relay under vz) starts; it takes no
commands from `warden`, so it keeps `warden-io-` and `warden-relay-` and
is not a `warden-driver-`. The `.exe` is part of the name on every host.

Container images are not in the list (Decision 6).

### The lookup

For one artifact, resolved once per `warden` invocation to absolute
paths:

1. **`PATH`.** Each entry in `PATH`'s order. The first directory that
   holds a regular file (symbolic links resolved) for every file name of
   the artifact is *the `PATH` candidate*, and the search of `PATH` ends
   there. If it passes the artifact's check, it is used.
2. **Beside `warden`**, when `PATH` held no candidate or its candidate
   did not pass. If the files are there and pass, they are used.
3. Otherwise an error that names the artifact, both places, what was
   found in each and why it was not used, and the fix (**What stops
   working**).

The details, each the same as K1's lookup unless it says otherwise:

- **A set comes from one directory.** A kernel from one directory is
  never booted with an initramfs from another (`relayvm.Find`'s rule
  today). A directory with some of a set's files is not a candidate.
- **Identity.** A `PATH` candidate that is the same directory as
  `warden`'s own (`os.SameFile` on the files) is the copy beside
  `warden`, reported as `beside-warden`, and looked at once. An install
  nobody has touched reports `beside-warden` for everything.
- **A relative `PATH` entry, or on Unix an empty one.** If such an entry
  holds the files, the lookup stops with an error that names the entry
  and the file; it neither uses them nor goes on. It is K1's rule and Go's
  (`exec.ErrDot`): the current directory may not be the user's own. On
  Windows the current directory is not searched before `PATH`, and an
  empty entry is skipped.
- **Differs from K1: no execute permission is asked for.** The host
  runs none of these files except the relay under vz, which `warden`
  copies into the work directory and makes executable. A directory in
  place of a file is not a candidate.
- **Differs from K1: a check, and the second place when it fails.**
  K1's first candidate is the driver, whatever it is. Here a `PATH`
  candidate that does not pass is passed over, and that is said on
  stderr, on every run, when the copy beside `warden` is then used:

  ```
  [warden] warden-io-linux-amd64: /home/ana/bin/warden-io-linux-amd64 is 0.6.0, below this warden's 0.7.0; using /home/ana/.local/bin/warden-io-linux-amd64 (0.7.0)
  ```

- **Differs from K1: no path form.** There is no `--driver <path>`
  equivalent. A user who wants another file puts it on `PATH`, where it
  wins when it passes the check.
- **Newer is used.** A candidate above the minimum is used with no line
  of its own; `artifacts.json` and doctor show its version.
- **When.** A driver looks up everything the request needs before it
  creates anything: no VM, container, network or work directory exists
  when a lookup fails (UX-04, UX-08). Only what the run uses is looked
  up: a Hyper-V build never asks for a Linux executor.
- **Never run.** The lookup reads files and directories and runs
  nothing.

**The check**, by kind:

| Kind | Passes when |
|---|---|
| executor, relay | the file states a version (below) that is at or above `warden`'s; for a `dev` `warden`, exactly `dev` |
| boot assets with a stamp | every file is present, and the stamp holds the channel this `warden` needs (`driver.LauncherStamp`, as today: `buildlink.Channel`) |
| boot assets with no stamp (qemu's relay VM kernel and base initramfs) | every file is present |

**Recorded.** When a build ends, successfully or not, `warden` writes
`<output>/artifacts.json` from the lookups the build made:

```json
{"warden": "0.7.0",
 "artifacts": [
   {"name": "warden-io-linux-amd64", "kind": "executor",
    "path": "/home/ana/.local/bin/warden-io-linux-amd64",
    "version": "0.7.0", "sha256": "9f2c...", "found": "beside-warden",
    "passed_over": [{"path": "/home/ana/bin/warden-io-linux-amd64",
                     "found": "PATH", "reason": "is 0.6.0, below 0.7.0"}]}]}
```

`found` is `PATH` or `beside-warden`. A boot-asset set has one entry per
file, with the stamp's value in place of `version`. The file is unsigned
and not part of the ledger, like K1's `driver.json`; it is the
machine-readable form (UX-02). `warden doctor --driver <name>` prints the
same for every artifact the driver can need, in a block named
`Artifacts`, and fails on one that every build of the driver needs and
that cannot be used. `warden image prepare --driver vz`, which is not a
build, prints one line naming the executor it installs.

### The version

**What states it.** Each of the four programs built from `cmd/`
(`warden`, `warden-io`, `relay`, `warden-pause`) links one small package
whose only content is one string variable with no initial value. The
build tooling sets it when it links the binary (`-X`), `make build` to
`dev` unless `VERSION` is given and goreleaser to the release's version:

```
buildwarden-version=0.7.0;
```

`warden --version` prints the part between `=` and `;`, and that
replaces `main.version`, so a binary has one version. `warden` reads an
artifact's version by scanning the file for `buildwarden-version=`
followed by 1 to 64 characters of `[0-9A-Za-z.+-]` and a `;`. The string
is ordinary read-only data in an ELF, PE or Mach-O file, so the scan is
the same for every OS and architecture and for a binary in any language.
Tried for this design in a scratch module, with `-trimpath -ldflags "-s
-w -X ..."`, for linux/amd64, linux/arm64, windows/amd64, windows/arm64
and darwin/arm64: each binary holds the line exactly once. Two things
the trial showed, which the design follows: the variable must have no
initial value (with one, the binary holds the initial line and the set
one), and the package that holds the variable must not spell the prefix
itself, so it lives apart from the scanner. A file with no such line, or
with two different ones, states no version and does not pass; that
includes a binary built with a bare `go build`. A `warden` built that way
calls itself `dev`.

**What the minimum is.** The version of the running `warden`, from the
same string. Nothing else is kept in code.

**How versions compare.**

| `warden` | Artifact | Passes |
|---|---|---|
| a release, `X.Y.Z` | a release at or above it, by Semantic Versioning 2.0.0 precedence (build metadata ignored) | yes |
| a release | an older release, `dev`, or no version | no |
| `dev` | `dev` | yes |
| `dev` | a release, or no version | no |

`dev` is a development build: what `make build` produces without a
`VERSION`. It has no place in the order, so it matches only itself. A
developer's `dist/warden` therefore never takes an installed release's
executor from `PATH`, and an installed release never takes a development
executor someone left on `PATH`. To make a customized artifact win, build
it with a version at or above the `warden` it is for (`make build
VERSION=0.7.0`).

**What this does not detect.** Two development builds are both `dev`, so
a `dist/warden-io-linux-amd64` left from an older commit beside a freshly
built `dist/warden` is used. On `main` the cache rebuilt it when `warden`
was newer. Keeping `dist/` whole is `make build`'s job now, which is what
"purely development processes" asks.

**BOOT-02.** On `main`: "delivered per build and matches the
orchestrator's version". After this item the executor is at or above the
orchestrator's version, and newer when the user has put a newer one on
`PATH`. That is a change to an invariant's wording, so it is a rule
change with a Bootstrap comparison (BOOT-07). Deviation D2 (vz bakes the
executor at prepare) is unchanged: the file baked is the one the lookup
found at prepare time.

### The relay image, the holder and the sidecar

| Image | A release (`warden --version` is not `dev`) | A development `warden` | Set by the user |
|---|---|---|---|
| relay | pulls `ghcr.io/buildwarden/relay:latest`, as today | `warden-relay:dev` from the runtime's local images | `relay_image` / `WARDEN_IMAGE`: a reference is pulled, as today; `dev` is the local `warden-relay:dev` |
| holder | the digest compiled in, pulled when absent, as today | `warden-holder:dev`, local | nothing |
| sidecar | the digest compiled in, as today | `warden-netns:dev`, local | nothing |

A `:dev` image is looked for with `<runtime> image inspect` and is never
pulled and never built by `warden`. When it is absent the build stops
before anything is created (**What stops working**, 1). `warden` no longer
embeds the two Containerfiles, removes no relay image after a build, and
tags nothing by content hash. `make images` builds the relay image from
`Dockerfile.relay`, the file the published image is built from, in place
of the inline Containerfile on `alpine:latest`.

The executor copied into the build container is `warden-io-linux-<host
arch>` from the lookup, for a release and a development build alike.

### Development tooling

- **`make build`** builds, with the version string, `dist/warden` and
  beside it: the Linux executor and relay for amd64 and arm64 (as today),
  the Windows executor for amd64 and arm64 (new: Hyper-V and qemu Windows
  guests asked `GuestBinary` to build it), and on macOS arm64 the two
  macOS binaries (as today). It then copies each boot asset the tree has
  built from `tools/*/output/` to `dist/` under its release name, with
  its stamp as `<name>.link-channel`. An asset that has not been built is
  not copied and not an error. `VERSION` is `dev` unless given.
- **`make images CTR=<runtime>`** (new) builds `warden-relay:dev`,
  `warden-holder:dev` and `warden-netns:dev` for the host's architecture
  in that runtime's local store, from `Dockerfile.relay` and the two
  Containerfiles under `driver/container/images/`. `CTR` defaults to the
  first of `finch`, `docker`, `podman` on `PATH`, the order `warden`
  detects in. It is separate from `make build` because it needs a
  running container runtime.
- **The asset scripts** keep writing `tools/*/output/`. Their closing
  lines and `warden`'s fix messages say to run `make build` afterwards.
- **`go run ./cmd/warden`** runs a `warden` in a temporary directory with
  nothing beside it. It finds artifacts only on `PATH`. It is no longer a
  way to run a build from a checkout; `dist/warden` is, as
  `CONTRIBUTING.md`, CI and the native sessions already do. A **`go
  install`**ed development `warden` works when `dist/` is on `PATH` or
  the artifacts are installed beside it.
- **Tests.** Four tests cross-build `warden-io` through `GuestBinary`
  today: `TestStageSeedDir`, `TestStageSeedDirConfiguredLink`
  (`driver/hyperv/winseed_test.go`), `TestWindowsSeedDirRunScript`
  (`driver/qemu/windows_test.go`) and `TestDescribeBuildEvalImage`
  (`driver/hyperv/environment_test.go`). Each only needs a file to stage.
  `driver` gains one documented, test-only function that points the
  lookup at a directory of the test's own and accepts what is there; the
  tests write a placeholder file. It replaces `project-config-trust`'s
  test-only setter of the tree and its two `TestMain`s. No test compiles
  anything, and `make test` needs no `dist/`.
- **CI.** The `ci` workflow's build job runs `make build` and then
  checks that every executable in `dist/` states the version
  `dist/warden` does. `test-integration.sh` runs `make build images`, so
  the `integration` workflow exercises the development images on docker.
  The `firecracker` workflow already runs the asset scripts and then
  `make build`, which now places the assets; its cache of
  `tools/*/output/` is unchanged.
- **Native sessions** build with `make build` in their checkout, as
  today; a session that validates the container driver also runs `make
  images`. The skill is not edited by this item (FLOW-14): its text says
  `make build`, which stays true, and the cases in the queue steps name
  `make images`.
- **Leftovers.** The per-tree cache `CacheDir()/bin/` is no longer read
  or written. `warden clean --cache` removes it, as it removed the flat
  cache before.

### Releases and install

- **`.goreleaser.yaml`.** The hook builds each executable with the
  version string and adds the Windows executor. Each archive carries
  `warden` and, under the names of **The artifacts**: the Linux executor
  and relay and the Windows executor for the archive's architecture, and
  in the macOS arm64 archive the two macOS binaries as well. The files
  `warden-io` and `warden-relay` are no longer in it, and the macOS
  binaries are no longer in the Linux archives.
- **`install.sh`** installs `warden` and every artifact in the archive
  into `INSTALL_DIR`. An older `warden-io` there is left alone.
- **What an installed release then runs with nothing built:** the
  container driver, as today. For qemu and Firecracker it has the relay
  and the executor; the boot assets are still to be built (a5). vz has
  everything once a macOS arm64 archive can be built. Hyper-V has no
  release.
- A tier-0 test reads the artifact list from the code and checks that
  the Makefile, `.goreleaser.yaml` and `install.sh` build, ship and
  install those names, the way `TestStampScriptsWriteChannel` holds the
  asset scripts to the stamp.

### What stops working, and what it says

Against `main` at `ad4ea50`. `project-config-trust` already ends cases 5
and 6 for a `warden` outside its tree, with messages this item replaces.
Every message is a `driver.FixableError` (UX-04) raised before anything
is created.

1. **A development `warden`, container driver, no `make images`.** On
   `main` the images are built per build.

   ```
   ERROR: relay image warden-relay:dev is not among docker's local images
     To fix: build the development images in warden's source tree: make images CTR=docker
   ```

   The same for `warden-holder:dev` and `warden-netns:dev`.

2. **A development `warden` whose `dist/` lacks an executable** `make
   build` did not use to build (the Windows executor), or was made by an
   older Makefile. On `main` it is built on first use.

   ```
   ERROR: executor warden-io-windows-amd64.exe not found (searched PATH, then C:\src\buildwarden\dist, where warden is)
     To fix: run 'make build' in warden's source tree, which builds it beside dist/warden
   ```

3. **A development `warden` with boot assets only in `tools/*/output/`.**
   On `main` they are found in the tree.

   ```
   ERROR: qemu relay VM assets (relay-vm-vmlinuz, relay-vm-initramfs.cpio.gz) not found (searched PATH, then /src/buildwarden/dist, where warden is)
     To fix: build them with tools/relay-vm/build-initramfs.sh, then run 'make build', which places them beside dist/warden
   ```

   For a release the fix reads: `no release ships them yet: in a
   buildwarden source tree, run tools/relay-vm/build-initramfs.sh and
   'make build', then copy dist/relay-vm-vmlinuz and
   dist/relay-vm-initramfs.cpio.gz beside warden`. Firecracker's and the
   direct-boot set's messages name their own files and scripts. A set
   whose stamp is wrong keeps today's sentence ("was built for an older
   warden and must be rebuilt once") with the same fix.

4. **A `warden` rebuilt alone, beside older executables of another
   version.** On `main` the cache rebuilt them. Two development builds
   are both `dev` and are not told apart (**The version**); a release
   beside older files says:

   ```
   ERROR: executor warden-io-linux-amd64 is too old for this warden (0.7.0)
            /home/ana/bin/warden-io-linux-amd64 is 0.6.0 (PATH)
            /home/ana/.local/bin/warden-io-linux-amd64 is 0.6.2 (beside warden)
     To fix: install this release's files beside warden (re-run install.sh, or copy every file of the release archive), or put warden-io-linux-amd64 0.7.0 or later on PATH
   ```

   A file that states no version reads `states no version`; a release
   file met by a development `warden` reads `is 0.6.0, and a development
   warden uses only development builds`, with the fix of case 2.

5. **`go run ./cmd/warden build`, and a `go install`ed development
   `warden`**, in a checkout. On `main` the working directory's tree is
   built. Now the message of case 2, unless the artifacts are on `PATH`.

6. **An installed release used inside a buildwarden checkout with a VM
   driver or `relay_image = "dev"`.** On `main` this compiles the
   checkout: the hole. Now a VM driver uses the release's own executables
   and gets case 3 for the boot assets, and `relay_image = "dev"` gets:

   ```
   ERROR: relay_image = "dev" (./warden.toml) names the development relay image warden-relay:dev, which is not among docker's local images
     To fix: remove relay_image to use this release's relay, or build the image in a buildwarden source tree: make images CTR=docker
   ```

7. **A release upgraded by copying only `warden` over an older install.**
   On `main` the new `warden` silently runs the old `warden-io`. Now, and
   until 1.0 (`TRANSITIONAL(artifact-names)`):

   ```
   ERROR: executor warden-io-linux-amd64 not found (searched PATH, then /usr/local/bin, where warden is installed); /usr/local/bin/warden-io is an older install's executor, which this warden does not use
     To fix: install this release's files beside warden: re-run install.sh, or copy every file of the release archive
   ```

   After the marker is removed the first sentence stands alone.

8. **A file of an artifact's name reached through a relative `PATH`
   entry.** On `main` nothing looks at `PATH`.

   ```
   ERROR: warden-io-linux-amd64 was found through the relative PATH entry "bin" (/src/app/bin/warden-io-linux-amd64), which warden does not use
     To fix: make the PATH entry absolute, or remove it
   ```

Nothing else stops: a release installed by `install.sh` runs the
container driver as before, and a tree built with `make build` (and `make
images` for the container driver) runs every driver it ran.

### Reference docs (UX-01) and the overview (FLOW-15)

- `docs/book/src/guide/configuration.md`: the `relay_image` comment and
  the **Relay Image** section (`dev` is the local development image; a
  development build uses it by default; nothing is compiled).
- `docs/book/src/guide/drivers.md`: a new section, **Artifacts**: the
  list, the lookup, the version, `artifacts.json`, what a file on `PATH`
  does and how to see it; the Firecracker **Boot assets** paragraph and
  the Hyper-V notes say `make build` places the assets.
- `docs/book/src/guide/cli.md`: `warden doctor`'s `Artifacts` block.
- `docs/book/src/getting-started/installation.md`: **One-liner Install**
  and **From Releases** (every file of the archive, and `~/.local/bin`,
  which is where `install.sh` installs; the page says `/usr/local/bin`
  and to move `warden` alone, which leaves the container driver without
  its executor today); **From Source** (the table of `dist/`, `make
  images`); the first-time QEMU, Firecracker and Hyper-V sections.
- `README.md` (the `relay_image` comment) and `CONTRIBUTING.md` ("Local
  builds report `version = "dev"`, which builds the relay from source"
  becomes `make build images`).
- `docs/book/src/contributing/structure.md`: the new package.
- `docs/book/src/concepts/overview.md` gains, under the orchestrator:

  ```
  find(artifact):                      # an executor, a relay, a boot-asset set
      for dir in PATH, in order:
          dir holds every file of the artifact:
              reached through a relative entry -> error
              check passes -> use it
              else         -> remember why; stop searching PATH
      beside warden holds them and the check passes -> use it (say what was passed over)
      else -> error: what was looked for, where, what was found, the fix

  check: executor, relay -> the version the file states >= warden's (dev: only dev)
         boot assets     -> all files present; its stamp, where it has one
  ```

  and the container driver's lines say the development images come from
  `make images`.

### How it sits with the other items

**Order of merges:** `runtime-cli-project-path`, then
`project-config-trust`, then this item. K1 (`driver-contract`) waits only
for `project-config-trust` and is not held behind this item. That is why
this is its own item: as that design's a6 records Jeff's answer, the
small fix stays there "so the hole closes with its merge and K1 is not
held behind native validation". So this item and K1 may merge in either
order, and the second consolidates (FLOW-10).

**`project-config-trust`** (read at `d3efc4a`). This item deletes what it
adds for the tree: `ModuleRoot` with its fixable error, the test-only
setter of the tree, the two `main_test.go` files' use of it, its two
messages that say to run `make build` and use `dist/warden`, and its
sentences in `CONTRIBUTING.md`, the installation guide and the Relay
Image section. Its `stageTestSeed` that fails where it skipped stays. Its
tier-1 case and corpus entry for F9 ("a `warden` outside any source tree,
run in a repository that declares the module, runs no `go`") must pass on
this branch, and this item widens the property: no `warden`, inside a
tree or not, runs `go`. How this item is held behind it is a8.

**`runtime-cli-project-path`** (approved at `a2afd08`). Nothing shared
but the convention: errors at once, transitional detection under a marker
until 1.0. UX-08 as it amends it is what **When** in the lookup follows.

**`driver-contract` (K1)** (read at `a6c8f0b`). K1's design describes
`GuestBinary` and `relayvm.Find` as they are on `main` and wraps them
(`relaysource.Bundled`, with a `Source` path in the tree and a `GoBuild`).

- *If this item merges first*, K1 inherits: the lookup in place of
  `GuestBinary`; `Bundled` without `Source` and `GoBuild`; its "Kept,
  existing: beside the binary, else the source tree" entry replaced by
  this design's named fallback; `Relay.Version` from the version string
  where it said `"dev"` for a source-tree build; and BOOT-02's sentence
  as amended here, on which its own amendment is re-quoted: "at or above
  the version of the driver that delivers it". For a driver binary "the
  executable" is the driver, as K1 already says, so "beside `warden`"
  becomes "beside the driver" and the minimum is the driver's version. Its
  `driver.json` and this item's `artifacts.json` sit side by side in the
  output; K1 may fold one into the other.
- *If K1 merges first*, this item's consolidation makes the same edits
  inside `driver/relaysource` and the container driver's binary, and
  re-quotes its BOOT-02 wording on K1's.
- Either way the two BOOT-02 amendments touch one sentence. This
  design gives its wording in both settings (**Rule changes**), so that
  its review clears the edit whichever of the two is on `main` when
  Implement makes it. K1's wording on top of this item's is K1's to put
  to its own review.

**K2** makes the VM drivers binaries; they keep the lookup as K1's
container driver has it. **K3 and K4** give each driver its own
repository and version: "at or above the driver's own version" stops
meaning anything for the kit's executor, and the driver then states a
minimum for it in code (a2's option (C), at that time). **Track M and
seam 8** (relay release forms with published hashes): each "adopt relay
version X" item (M6) replaces `warden-relay-*` and the relay's boot
assets for one driver with a relay release, and how a relay states its
version becomes seam 8's to say; the version line here is one any
language can carry. **`seam-specs`** (read at `defdb24`): its `make
conformance` builds the relay and the collector it tests into
`dist/conformance/`, which is development tooling and no part of what
`warden` looks up; both items add targets to the `Makefile`, in
different lines.

### Handed on

Findings that are not this item's. This agent edits only its own queue
item, so the session that records this design's review files them.

| What | Where it goes |
|---|---|
| goreleaser's macOS arm64 build of `warden` (`CGO_ENABLED=0`) does not compile at `ad4ea50`; K1's design lists it as "to be filed" | a fix item through intake |
| The release default relay image is the moving tag `:latest` (a4) | seam 8 / M6 |
| Boot assets in a release (a5, option (B)) | track M for the relay's, K3 and K4 for a driver's |
| The corpus entries of the Security note | `security-suite` (SEC-03), at Publish |
| Removing `TRANSITIONAL(artifact-names)` at 1.0 | its own small item, filed at Publish |

## Defaults and overrides

| Default or fallback | Why it is the obvious one | What overrides it |
|---|---|---|
| **New, named fallback: `PATH`, then beside `warden`.** Triggered when `PATH` holds no candidate, or its first candidate does not pass the check | Jeff's sentence. A file on `PATH` is where a user puts programs; the copy beside `warden` is the one it shipped with | Nothing names a file. A copy on `PATH` that passes wins. Seen as: the passed-over line on stderr on every run, `artifacts.json`, doctor |
| **New: the minimum is `warden`'s own version**; `dev` takes only `dev` | Everything is built from one tree and released together; no number is kept by hand | None |
| **Changed: a development `warden`'s relay, holder and sidecar images** are `warden-relay:dev`, `warden-holder:dev`, `warden-netns:dev`, local | They are what `make images` builds; `warden` no longer builds any | The relay: `relay_image` / `WARDEN_IMAGE`, used exclusively. The other two: none |
| **Changed: `relay_image = "dev"`** is the local image `warden-relay:dev`, not a build | The value keeps its name and its intent, "my development relay" | Any other value |
| **Kept: a release's images** (the relay from `:latest`, the holder and sidecar by digest) | As on `main` | `relay_image` for the relay |
| **Kept: Hyper-V's relay boot VHDX** in the hyperv cache | Not compiled by `warden` and not looked for in a tree | `WARDEN_RELAY_BOOT_VHDX` |
| **Removed fallbacks:** the build from a source tree when a file is not beside `warden`; the tree's `tools/*/output/` when assets are not beside `warden`; the build of `warden-io` when a release does not find it | Each ran only when the first place failed, and each compiled or trusted a tree (Decision 1) | |

No merge is added. A provided `relay_image` that cannot be used is an
error that names it; the default is not tried (GLOBAL-10).

## Cross-driver impact

| Driver | What changes |
|---|---|
| container | The executor comes from the lookup (`warden-io-linux-<arch>`) for every version. A development build uses the three `:dev` images and builds none; `relay_image = "dev"` likewise. `goBuildLinux`, `buildRelayFromSource`, the embedded Containerfiles and the content-hash tags go. A release's pulls are unchanged |
| qemu | The relay binary, the relay VM set, the direct-boot set and the guest's executor come from the lookup, all before anything boots. Direct boot appends the executor per build (Decision 8) |
| firecracker | The relay binary, the three-file set and the executor come from the lookup. One set, one directory, as its single `relayvm.Find` has it today |
| vz | The relay it runs on the host and the executor `image prepare` installs come from the lookup. Under `sudo`, `PATH` is sudo's and the copy beside `warden` is what is usually found |
| hyperv | The Windows executor on the seed comes from the lookup. The relay boot VHDX and its stamp are untouched |

No driver is unaffected. The lookup, the version reader and the recorder
are one implementation in `driver/`, used by all five (ORCH-01).

## Specification changes

| Seam | Specification | Change |
|---|---|---|
| `bootstrap-model` | No written specification. It is defined by `docs/development/bootstrap.md` (the diagram, BOOT-01 to BOOT-06, the mapping and the register) | BOOT-02's second sentence and the diagram's "version-matched" label, as in **Rule changes**, made in that file in Implement, before the code (LOOP-02). The qemu / Linux row of the mapping gains its direct-boot delivery, a correction to match the code as Decision 8 leaves it |

No specification under `specs/` is touched: the ledger and its metadata
schemas do not record the executor or the relay binary, before or after.

## Bootstrap comparison

Required because one invariant's wording changes (BOOT-07). The review
must accept this section explicitly.

### Current model

The diagram in [`bootstrap.md`](../development/bootstrap.md). The lines
that matter:

```
  HOST (warden + collector)                    BUILD ENVIRONMENT

  (2) deliver the executor ----------------->  warden-io + a launcher
      per build, version-matched               (seed disk, cloud-init ISO,
                                               copied-in file, ...)
```

### Proposed model

```
  HOST (warden + collector)                    BUILD ENVIRONMENT

  (2) deliver the executor ----------------->  warden-io + a launcher
      per build, at or above warden's version  (seed disk, cloud-init ISO,
                                               copied-in file, ...)
```

Everything else in the diagram stands. In the mapping, the qemu / Linux
row's delivery reads "cloud-init `CIDATA` ISO per build, or `warden-io`
appended per build to a launcher-only initramfs (direct boot)", which
corrects it to the code after Decision 8. The deviation register does not
change.

### Per invariant

| Invariant | Current | Proposed | Per-driver effect |
|---|---|---|---|
| BOOT-01 isolation precedes execution | As written | Unchanged | None. Lookups happen before anything is created |
| BOOT-02 the executor is the only out-of-band input | "It is delivered per build and matches the orchestrator's version." | "It is delivered per build, and its version is at or above the orchestrator's: an executor that states an older version, or none, is not delivered." | All five: the executor is the file the lookup found, the copy shipped with `warden` unless the user put one at or above its version on `PATH`. qemu direct boot: delivered per build, where it was the asset's. vz: D2 as before; the baked file is the one found at prepare |
| BOOT-03 one start, then hands off | As written | Unchanged | None |
| BOOT-04 build inputs come from the relay | As written | Unchanged | None |
| BOOT-05 setup is the executor's domain | As written | Unchanged | None |
| BOOT-06 images carry hooks, not content | As written | Unchanged. qemu's direct-boot initramfs stops carrying an executor tied to a `warden` version | qemu: the asset no longer needs rebuilding when `warden-io` changes |

### Gains

- The rule says what `warden` enforces. On `main` nothing checks that a
  release's `warden-io` beside it is its own; an upgrade that replaces
  only `warden` runs the old executor without a word.
- A user can run a newer or customized executor without rebuilding
  `warden`, the same freedom K1 gives for a driver.
- The source of the executor is a file with a version and a digest in the
  build's output, not a compile whose input was whatever tree was found.

### Costs

- **Security posture.** The executor is the one input outside the
  relay, and it is now taken from `PATH` first. Whoever can write a
  directory on the user's `PATH` can supply it. The Security note states
  this in the terms accepted for K1.
- **The record of what the build consumed.** Unchanged in the ledger:
  the executor was not recorded there and is not now.
  `artifacts.json` adds its path, version and digest, unsigned.
- **A newer executor than `warden` can run.** Before 1.0 the relay guest
  API and the `initialize` contract may still move, so a much newer
  executor may not work with an older `warden`'s relay. `warden` does not
  refuse it; the build's failure and `artifacts.json` show the versions.

## Rule changes

One entry. It is a change to a component's invariant, not to a loop rule
(LOOP-09).

### BOOT-02 (amend), `docs/development/bootstrap.md`

**Why:** Jeff's rule accepts an executor "on or above a minimum
version". "Matches the orchestrator's version" would be false whenever a
newer executor on `PATH` is used.

**Current wording** (the rule's body, on `main` at `ad4ea50`):

> The host delivers exactly one thing to the build environment outside the relay:
> the executor (`warden-io`) and the minimal launcher that starts it. It is
> delivered per build and matches the orchestrator's version. The build script,
> context files, the CA and extension material never travel this way.

**Proposed wording:**

> The host delivers exactly one thing to the build environment outside the relay:
> the executor (`warden-io`) and the minimal launcher that starts it. It is
> delivered per build, and its version is at or above the orchestrator's: an
> executor that states an older version, or none, is not delivered. The build
> script, context files, the CA and extension material never travel this way.

In the model diagram, `per build, version-matched` becomes `per build, at
or above warden's version`. In the per-driver mapping, the qemu / Linux
row's delivery cell becomes "cloud-init `CIDATA` ISO per build, or
`warden-io` appended per build to a launcher-only initramfs (direct
boot)", a correction to match the code. The deviation register does not
change.

**Severity and check:** MUST, `agent-judgment`, before and after. No
enforcement change: `tools/tier1/` is not edited. The `bootstrap-surface`
check sees the edit to `bootstrap.md` beside a design whose `rules` cites
BOOT-07.

**In the other setting.** K1 amends the same sentence, and its wording
was approved by name in its review (design at `a6c8f0b`; the review's
outcome is not yet recorded): "It is delivered per build by the driver
and matches the version of the driver that delivers it; a driver linked
into `warden` has `warden`'s version." If that is the sentence on `main`
when this item's Implement reaches it, the proposed wording is:

> The host delivers exactly one thing to the build environment outside the relay:
> the executor (`warden-io`) and the minimal launcher that starts it. It is
> delivered per build by the driver, and its version is at or above that of the
> driver that delivers it: an executor that states an older version, or none, is
> not delivered; a driver linked into `warden` has `warden`'s version. The build
> script, context files, the CA and extension material never travel this way.

and the diagram's label, which K1 makes `per build, the driver's
version`, becomes `per build, at or above the driver's version`. The
review is asked to approve the change in both settings, as K1's review
was for ORCH-03. If `main` then holds neither sentence as quoted, nothing
of BOOT-02 is edited and the item returns to Design (FLOW-06). If this
item merges first, K1's amendment is re-quoted on this wording by K1.

## Security note

**Boundary:** the host's choice of the code that runs as the relay and as
the executor. The relay is the build environment's only network path and
the signer of the ledger; under vz it runs on the host. The executor is
BOOT-02's one input that does not pass through the relay.

**How the posture changes.**

- **Closed: source from a repository is no longer compiled on the
  host.** On `main` a `warden` that lacks a file builds it from the tree
  it finds, and the working directory's tree qualifies. After
  `project-config-trust` only the tree around the executable does. After
  this item there is no tree and no compiler: `warden` starts no `go`,
  and builds no image, in any directory, for any version. A repository
  cannot supply source, and a `warden.toml` cannot ask for a build:
  `relay_image = "dev"` selects an image that must already be in the
  user's own image store.
- **Opened by decision: the files are taken from `PATH` first.** This
  is K1's posture, in Jeff's words for it: the lookup "is never outside
  of the user's control (i.e. never a part of the build environment)".
  Nothing inside the build environment can reach `PATH`, `warden`'s
  directory or the host's image store. Whoever can write a directory on
  the user's `PATH` can already replace `git`, the container runtime or
  `warden`, so the lookup gives such a writer no new way in; it gives
  them more files to replace.
- **What a planted file can do.** A file of an artifact's name in an
  absolute `PATH` directory, stating a version at or above `warden`'s,
  is used in place of the copy beside `warden`, wherever `warden`'s own
  directory is in `PATH`. A planted executor runs as the first process
  of the build environment and decides what the build does; a planted
  relay carries all of the build's traffic and signs the ledger, and
  under vz runs on the host as the user; a planted kernel or initramfs is
  the relay VM. The version check does not stop this and is not meant to:
  the line is a few bytes anyone can write. It is a check that the file
  fits, not that it is trusted.
- **What a build context can and cannot do.** It cannot make `warden`
  compile anything. It cannot name an artifact's file: no key, flag or
  variable does. A file it ships is not used from the working directory,
  nor through a relative or empty `PATH` entry, where the lookup stops
  with an error. It can supply an artifact only where the user's `PATH`
  holds an absolute entry inside the repository, which some setups make
  (a per-project `bin` added by an environment hook); K1's design says
  the same of a driver binary, and the reference says so (UX-01). It can
  still choose the relay image by reference through `./warden.toml`, as
  on `main`; that is `project-config-trust`'s subject and unchanged here.
- **What shows which file was used.** The passed-over line on stderr;
  `<output>/artifacts.json` with each file's path, version, digest and
  where it was found; `warden doctor --driver <name>`. A build's output
  therefore says that a file other than the bundled one was used exactly
  when `found` is `PATH`. None of this is signed.
- **`sudo`.** `sudo warden image prepare --driver vz` runs with the
  `PATH` sudo gives it, which by default is not the user's, so the copy
  beside `warden` is what is usually found there; a file in a directory
  of that `PATH` is root's to control.

**What the TEST-08 suite should exercise** (corpus entries, SEC-03):

1. A repository that declares the module and carries a modified
   `cmd/relay`, `cmd/warden-io` and `tools/relay-vm/output/`: a `warden`
   run there, with a `go` on `PATH` that records being called, for each
   driver that can run on the host and with `relay_image = "dev"`: `go`
   is never called, and no file of the repository is staged.
2. The same with `warden` built inside that repository's tree: still no
   `go`.
3. A repository that ships `warden-io-linux-<arch>` in its root and in
   `bin/`, with `PATH` holding `.`, an empty entry, or `bin`: the build is
   refused with the relative-entry error; with none of those in `PATH`
   the files are not used.
4. A `warden.toml` in the context with `relay_image = "dev"` and no local
   `warden-relay:dev`: refused before anything is created, nothing
   built.
5. A file on `PATH` that states an older version, and one that states
   none: passed over, with the line, and the bundled copy runs.

## Validation plan

**Tier 0** (`make test`, every platform; TEST-01).

- The lookup, table-driven, over temporary directories and a `PATH`
  given to the function: no candidate; the first candidate passes; it
  fails and the copy beside `warden` passes; both fail (the error names
  both); a later `PATH` copy is ignored; a relative and an empty entry;
  a set split across two directories is not a candidate; the same
  directory on `PATH` and beside `warden` is looked at once; Windows
  `PATH` splitting and the `.exe` names (build-tagged cases).
- The version reader over byte slices: the line present, absent,
  truncated, over-long, twice; and the comparison table of **The
  version**, including pre-release precedence.
- Each driver's staging with the test-only directory: the four tests
  named under **Development tooling** pass with a placeholder file and
  no compiler; a missing file is the fixable error, before any work
  directory exists.
- The container driver's image choice as a table over (version, 
  `relay_image`): which reference, and that no build is attempted; the
  errors of cases 1 and 6.
- `artifacts.json` round-trips, and the doctor block's lines.
- The transitional error (case 7) and its absence when the new name is
  found.
- The artifact list against the Makefile, `.goreleaser.yaml` and
  `install.sh`.
- No `exec.Command` of `go` remains under `cmd/` or `driver/` (a test
  that reads the tree, as the stamp-script test does).

**Tier 1** (the `integration` workflow, docker on Linux): `make build
images`, then the existing builds; a build with the executor copied to an
absolute `PATH` directory records `found: PATH`; with `warden-relay:dev`
removed, the error of case 1; `project-config-trust`'s F9 case, and the
same with `dist/warden` inside the tree, with a recording `go` first on
`PATH`. The `firecracker` workflow (tier 2 in CI, on pull requests that
reach `driver/*.go`): its builds pass with the assets placed by `make
build`, and its doctor shows the `Artifacts` block.

**Tier 2, native** (FLOW-09). Every driver's tree is reached, so each
host whose driver only it can run blocks:

| Host | Blocks | Cases |
|---|---|---|
| `macos-finch` | yes | `make build`, `make images CTR=finch`; doctor's block; a default build verifies and `artifacts.json` says `beside-warden`; no `go` is started (a recording `go` on `PATH`); with `warden-relay:dev` removed, case 1's error before anything is created; a copy of the executor on `PATH` is used, one built with an older `VERSION` is passed over with the line; `relay_image = "dev"` and `WARDEN_IMAGE=<ref>` |
| `macos-vz` | yes | `sudo dist/warden image prepare --driver vz` installs `dist/warden-io-darwin-arm64` and names it; a build runs a copy of `dist/warden-relay-darwin-arm64`; with that file moved away, refused before any VM boots; no `go` is started |
| `windows-hyperv` | yes | `make build` under Git Bash makes `dist/warden-io-windows-amd64.exe`; a durable and an ephemeral build pass, and the seed's `warden-io.exe` has the digest in `artifacts.json`; with the file moved away, case 2's error before any VM; a copy in a `PATH` directory is used and one in the working directory is not; `go test ./driver/...` on Windows; the relay boot VHDX is found as before |
| `qemu-any` | yes | the asset scripts, then `make build` places both sets with the stamp; a direct-boot build and a cloud-image build verify; the direct-boot build runs the appended executor (its digest in `artifacts.json`), with an initramfs that still carries a baked one as well as a rebuilt one; assets moved away give case 3 before any VM; run from a directory that is a buildwarden checkout with `tools/relay-vm/output/` filled, nothing of it is used |
| `container-any` | no, tracked | the Windows desktop: `go test`, `make images` under Git Bash, a container build (`prebuilt-artifacts-windows`) |
| `macos-qemu` | no, tracked | a qemu Windows guest build with the Windows executor from the lookup (`prebuilt-artifacts-qemu-windows`) |

Firecracker has no native host that is not experimental (`linux-kvm`);
its CI workflow is its tier 2.

**Not run for this design.** Read and not run: every driver's build, the
asset scripts, goreleaser, `install.sh`, any container runtime, macOS and
Windows. Run: the cross-builds quoted (`GOOS=darwin GOARCH=arm64
CGO_ENABLED=0 go build ./cmd/warden` fails; Windows and Linux arm64
build), and `go version -m` on a Windows and a macOS binary built in this
worktree, and `go run`, `go build`, `go test` and `-buildvcs=false` in a
scratch module, for Decision 4's rejected alternative; and the version
line, set with `-X` and found with `grep -a`, in binaries of a scratch
module for five OS and architecture pairs (**The version**). Not tried:
the line in this repository's own four programs, which is Implement's
second task; if a binary there does not hold it whole and once, that is a
finding for Design (FLOW-06), and a3's option (B) is the known
alternative.

## Implementation plan

Plan starts after `project-config-trust` has merged (a8) and merges
`main` first. Rule-file edits come first (LOOP-02).

1. `docs/development/bootstrap.md`: BOOT-02, the diagram label and the
   qemu / Linux mapping cell, as approved (FLOW-14, BOOT-07).
2. The version package and the linker flag in the Makefile; `warden
   --version` reads it; a test that builds nothing reads the line from a
   byte slice, and the CI build job proves it on real `dist/` binaries
   for Linux, Windows and macOS targets (GLOBAL-04, GLOBAL-07).
3. The lookup, the check, the recorder and the test-only directory in
   `driver/`, with the tier-0 table (GLOBAL-03, GLOBAL-10, GLOBAL-11,
   UX-04).
4. Rework: every `GuestBinary` caller (rows 2a to 2g) asks the lookup,
   before anything is created (ORCH-01, UX-08). `relayvm.Find` and
   `StampPath` lose the tree; the qemu and Firecracker fix messages are
   rewritten.
5. Rework: the container driver: the executor from the lookup, the
   `:dev` images, `relay_image = "dev"`; delete `goBuildLinux`,
   `buildRelayFromSource`, the embedded Containerfiles and the image
   removal (GLOBAL-09, UX-03).
6. Rework: qemu direct boot appends the executor;
   `tools/build-vm/build-initramfs.sh` stops compiling it (BOOT-02,
   BOOT-06).
7. Delete `ModuleRoot`, `GuestBinary`, the cache helpers, and
   `project-config-trust`'s setter and `TestMain`s; move the four tests
   to the test-only directory (GLOBAL-04).
8. `artifacts.json`, the doctor block, the passed-over line, the vz
   prepare line (UX-02).
9. The transitional detection, in one file under
   `TRANSITIONAL(artifact-names)` (UX-03).
10. `make build` (Windows executors, the asset copy), `make images`,
    `test-integration.sh`, the `ci` build job's version check
    (TEST-02).
11. `.goreleaser.yaml` and `install.sh`, with the tier-0 test that holds
    them to the artifact list.
12. Reference docs, `README.md`, `CONTRIBUTING.md`, `structure.md` and
    the overview (UX-01, FLOW-15).
13. `make test`, `make lint` and `GOOS=windows`, `GOOS=darwin make lint`
    (FLOW-17).

## Design review

No review yet.

## Consolidation

Filled in at Publish.

## Rollback notes

None yet.

## Archive summary

Filled in at Publish.
