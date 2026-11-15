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
                         # mean. output: <output>/host-files.json, a Host files block in warden
                         # doctor, one line when a file on PATH is passed over, new errors
security_boundary: host-env  # which files the host runs as the relay (vz runs it on the host)
                         # and delivers as the executor, the one input outside the relay. It
                         # also reaches env-relay: the relay binary, image and boot assets
alters_relationships: true  # BOOT-02 is amended, and the bootstrap model is one of FLOW-02's
                         # grounds. It is the only ground here: compilation leaves the
                         # components for development tooling, and no component takes a
                         # responsibility from another (Design, The front matter, argued)
design_review: required  # FLOW-19: a security boundary, a seam, a rule change, blocking native
                         # validation and assumed questions
native_validation:
  - {host: macos-finch, blocking: true, reason: "driver/container is reached: a development warden no longer builds the relay, holder and sidecar images or the executor per build; it runs the images make images built and recorded beside it, in finch's store, which CI (docker on Linux) does not exercise"}
  - {host: macos-vz, blocking: true, reason: "driver/vz is reached: the relay it runs on the host and the executor image prepare installs come from the lookup, under sudo's PATH for prepare; only this host runs them"}
  - {host: windows-hyperv, blocking: true, reason: "driver/hyperv is reached: the Windows executor on the seed comes from the lookup, and the lookup's PATH handling on Windows (';', no current directory) runs with a real build only here"}
  - {host: qemu-any, blocking: true, reason: "driver/qemu is reached: the relay binary, the relay and direct-boot assets come from the lookup, and direct boot starts delivering the executor per build; CI runs no qemu build"}
  - {host: container-any, blocking: false, reason: "the container driver on a Windows host runs the same code CI runs on Linux; what only this host adds is make images under Git Bash (a build context with no exec bit) and go test ./driver ./cmd/warden on Windows, which CI does not run"}
  - {host: macos-qemu, blocking: false, reason: "a qemu Windows guest takes warden-io-windows-arm64.exe from the lookup; qemu Windows builds do not complete on main (backlog qemu-windows-guest-package)"}
tier2_gaps:
  - {host: container-any, gap: "on the Windows desktop: go test ./driver/... ./cmd/warden; make build and make images CTR=<runtime> under Git Bash; the relay container made from the development image starts (its relay is executable); a default container build with the development images; a copy of the executor in a PATH directory is used and one in the working directory is not (backlog prebuilt-artifacts-windows)"}
  - {host: macos-qemu, gap: "a qemu Windows guest build (--guest-os windows) stages dist/warden-io-windows-arm64.exe found by the lookup, and is refused before any VM boots when the file is missing (backlog prebuilt-artifacts-qemu-windows)"}
rules: [GLOBAL-01, GLOBAL-03, GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, GLOBAL-09, GLOBAL-10,
        GLOBAL-11, ORCH-01, ORCH-03, BOOT-01, BOOT-02, BOOT-06, BOOT-07, RELAY-08, UX-01, UX-02,
        UX-03, UX-04, UX-07, UX-08, TEST-01, TEST-02, TEST-03, TEST-04, TEST-06, TEST-08,
        SEC-03, FLOW-01, FLOW-02, FLOW-05, FLOW-06, FLOW-07, FLOW-09, FLOW-10, FLOW-12,
        FLOW-14, FLOW-15, FLOW-16, FLOW-17, FLOW-19, FLOW-20, LOOP-02, LOOP-04, LOOP-09]
rule_changes:
  - {rule: BOOT-02, change: amend, paths: [docs/development/bootstrap.md]}
concepts: [prebuilt-artifacts, artifact-lookup, artifact-version, minimum-version, guest-binary,
           executor-delivery, relay-binary, relay-vm-assets, boot-assets, relay-image,
           dev-build, dev-images, host-files, module-root, source-tree, release-archive,
           install-script, goreleaser, transitional-detection, security-fix]
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
`project-config-trust`'s (**F9: the source tree**, read at `0bda630`): a
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
  build cache by source tree so that two checkouts on one machine stop
  staging each other's builds. Its Decision 2 says "a release ships its
  guest binaries next to `warden` (lookup 1), so it never relied on the
  cache". That is not what a release does: the one published archive
  layout holds `warden` and `warden-io`, and the lookup asks for
  `warden-io-linux-<arch>` (**Inventory**). This item removes the cache
  and the build with it, and keeps that design's property for the one
  thing that is still shared by every checkout on a machine, the
  container runtime's image store (Decision 6).
- **`build-link-subnet`** and **`hyperv-relay-per-build-channel`**
  (archived). They gave each launcher-carrying boot asset a stamp file
  beside it (`<file>.link-channel`, `WARDEN_LINK_CHANNEL=<n>`), written
  by the tool that builds the asset and compared by `warden` before a
  build that uses the asset (`driver.LauncherStamp`). This item keeps the
  stamp, its values, when it is checked and its messages, and makes it
  the check a candidate must pass.
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
- **`driver-contract`** (K1). Approved on 2026-11-15 at `a6c8f0b`, with
  eleven rule changes approved by name, BOOT-02 among them (its branch
  read at `c767992`, which records the approval). Its Plan is held until
  `project-config-trust` has merged. **Discovery and trust** and its
  Security note carry Jeff's answer to its a16, the model for this
  lookup: `PATH` wins over a bundled copy, then `warden`'s own directory;
  a file reached through the working directory or a relative `PATH`
  entry is not used; the only guard is that no config file names the
  file by path; heavier guarding is "overcontrolled ... this is never
  outside of the user's control (i.e. never a part of the build
  environment)". **The lookup** says where this one differs and why.
- **`project-config-trust`** (in design review; read at `0bda630`) and
  **`runtime-cli-project-path`** (approved at `a2afd08`; in Publish,
  branch read at `9cb1dc0`): one `warden.toml` per build; a setting is
  validated by its consumer at initialization, after the overrides, only
  for what the run uses (UX-08 as that item amends it); a change that
  refuses what worked before is an error at once with a concrete message,
  and the code that detects the old state is kept until 1.0 under a
  `TRANSITIONAL(<name>)` marker.
- **`seam-specs`** (S1; approved at `64a4a95`, branch read at
  `b230b83`). It specifies `executor-initialize` as "a command line, its
  steps and its exit" and gives a relay's command line to its harness as
  a file with placeholders; neither closes the list of a program's
  flags, so the `--version` this item adds to two programs changes no
  specification.
- A design named `baked-asset-staleness` is referred to by two backlog
  entries; it is not on `main` and nothing here relies on it.

## Decisions

Terms. An **artifact** is a file, or a set of files that go together,
that `warden` needs for a build and does not contain: an executor, a
relay binary, a boot-asset set. It is Jeff's word, and this design's; the
user-facing name is **host file**, because in the output directory, the
collector and the guide an *artifact* is already a build's output
(`artifacts/`). A **candidate** is a directory that holds every file of
the artifact; a stamp is not one of an artifact's files, it is what a
check reads. **Beside `warden`** is the directory of `warden`'s
executable, symbolic links resolved.

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
   an artifact by path. As in K1, `warden`'s own directory met on `PATH`
   is a candidate like any other and ends the search, so a copy in a
   `PATH` entry after it is never used.
   - *Rejected: every acceptable candidate on `PATH` in turn.* His words
     name two places. A second copy later on `PATH` is passed over, as
     K1's is.
   - *Rejected: a flag, variable or key that names an artifact's file.*
     K1's one guard is that no config file names a driver's file; here no
     source does, so there is nothing to guard.
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
   `warden` reads without running the file** (**The version**). Each
   program that is looked up prints the line when asked (`--version`),
   which is also what keeps the linker from dropping it, and every build
   uses `-trimpath`.
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

5. **A boot-asset set is checked by its stamp, where and when it is
   checked today, not by a version.** A kernel and an initramfs do not
   change with `warden`'s release (the relay is appended per build), and
   requiring `warden`'s version would mean rebuilding a kernel for every
   release. A directory is a candidate when it holds the set's kernel and
   initramfs files. The link-channel stamp is the check, with today's two
   failing states (missing, stale), and it is applied only to a build
   that boots the stamped file, which is direct boot: a Firecracker or
   qemu build from a disk image does not read it, on `main` or here. A
   set with no stamp defined (qemu's relay VM kernel and base initramfs)
   is accepted when complete (a5).

6. **Container images are named by reference, not found by the lookup,
   and a development `warden` runs the images its own checkout built.**
   A release pulls what it pulls today. `make images` builds the relay,
   holder and sidecar images, tags each by the hash of what it was built
   from, and writes the three references to
   `dist/warden-dev-images.json`; a `warden` that has no reference
   compiled in reads them from that file beside itself and from nowhere
   else. `relay_image = "dev"` keeps working and means "the development
   relay image recorded beside this `warden`" (a4).
   - *Rejected: three fixed tags* (`warden-relay:dev`,
     `warden-holder:dev`, `warden-netns:dev`), which the first draft of
     this design had. One set per container runtime is then shared by
     every checkout on the machine: branch A's `make images` replaces the
     images branch B's `dist/warden` runs. Native sessions test `main`
     and every waiting branch from worktrees on one machine, so a tier-2
     pass could be a run of another branch's relay. That is the hazard
     `guest-binary-cache-per-tree` removed for guest binaries. And
     `warden clean`, on `main` and in every release, removes any
     `warden-relay:<tag>` whose tag is not a live build ID
     (`cmd/warden/clean.go`), `dev` included; run for the review of this
     design, it printed `Removing image: warden-relay:dev`.
   - *Rejected: remove the value `dev`.* It is a documented value
     (UX-03) and still has a meaning.
   - *Rejected: copy the relay binary into a container made from the
     released image*, as the executor is copied into the build container.
     It would make the container driver's relay a file like every other
     driver's. It changes how a release's relay container is made, which
     is more than removing compilation, and the relay's end state is an
     OCI image released by the relay repository (seam 8).
   - *Rejected: a version label on images, checked against the
     minimum.* A provided reference is used as given (GLOBAL-10), and a
     release's references are compiled in. A development image cannot be
     stale under this decision: its tag is the hash of its relay binary
     and Containerfile, so a rebuilt relay names an image that does not
     exist until `make images` has run.

7. **Development tooling places everything beside `dist/warden`**
   (**Development tooling**). `make build` builds every executable the
   host's drivers need and copies the boot assets the tree has built into
   `dist/`; `make images` builds the three development images and
   records them there.

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
     have to except one more path from "the executor is the file the
     lookup found". It already excepts vz, which deviation D2 covers.

9. **A release ships each artifact under the name it is looked up by,
   and `install.sh` installs whatever the archive holds** (**Releases
   and install**). The names `warden-io` and `warden-relay` go from new
   archives. `install.sh` is fetched from `main` and installs the latest
   published release, or any version named, so it must install the
   archive of every published layout: it enumerates the archive's
   `warden*` files and names none. An old `warden-io` beside a new
   `warden` is detected and named in the error until 1.0
   (`TRANSITIONAL(artifact-names)`) (a7).
   - *Rejected: accept `warden-io` as a second name.* The file an older
     install left states no version and would be refused anyway.

10. **What was used is recorded, and what was checked is what is used.**
    Every build writes `<output>/host-files.json`: each file the lookup
    found, each container image with the ID the runtime resolved it to,
    and Hyper-V's relay boot disk. `warden doctor --driver <name>` prints
    the same as a `Host files` block. A file on `PATH` that is passed
    over is said on stderr on every run. Each file is read once into the
    build's work directory, and the check, the digest and every later use
    are of those bytes. No command is added.

11. **This item does not ship boot assets in a release, and adds no
    Windows release.** Neither exists today. The error for a missing
    boot-asset set says that no release ships it and how to build and
    place it (a5).

12. **The release configuration this item edits is made to run, and CI
    runs it.** `main`'s `.goreleaser.yaml` has never produced a release:
    its `before` hook cannot run as written, its output directory is one
    goreleaser refuses, and the macOS arm64 `warden` does not compile
    (**Inventory**). This item rewrites the hook and the archive's file
    list, so it fixes the first two, which are in the lines it rewrites,
    and adds a CI step that builds a snapshot release and checks every
    archive against the artifact list. The third is `main`'s regression
    and is filed as this item's follow-up (FLOW-09; **Handed on**).
    - *Rejected: change the file list and leave the hook.* A tier-0 test
      that reads the files would have passed the broken hook; nothing
      short of running goreleaser shows that an archive holds what the
      lookup asks for.

## Questions

### Blocking

None.

### Assumed

Most consequential first. Each is confirmed in the design review
(FLOW-19).

**a1. The lookup, made precise.** Jeff's sentence gives the two places
and the trigger. Proceeding with: the *first* candidate on `PATH`, and no
later one; `warden`'s own directory met on `PATH` is that candidate when
it comes first, so a copy in a later entry is not used; a candidate
reached through a relative or empty `PATH` entry stops the lookup with an
error, as K1's does; then beside `warden`; no flag, variable or key names
an artifact's file; the same lookup for a kernel and an initramfs as for
an executable; a newer artifact is used without comment; the record is
`<output>/host-files.json` and doctor's `Host files` block (not
`artifacts.json`: `artifacts/` is the build's outputs), with the
passed-over line on stderr. The lookup finds BuildWarden's own relay and
executor wherever they are installed; a different build of them on `PATH`
is used and recorded, and is not a supported substitution (GLOBAL-08
stands). Confirm.

- (A) as above. *Recommended*: it is K1's lookup plus exactly what the
  sentence adds.
- (B) every `PATH` entry in turn until one is acceptable, then beside
  `warden`. More forgiving, and which file wins is then harder to
  predict.
- (C) executables by this lookup; kernels and initramfs only beside
  `warden`. `PATH` then holds only programs, at the cost of a second
  rule.

**a9. The order of this item and K1 (`driver-contract`).** The two
rewrite the same code (`GuestBinary`'s callers, the container driver,
the relay's lookups) and the same sentence of BOOT-02, and K1 was
approved today on `main`'s sentence. Jeff's reason for making this its
own item was that K1 is not held behind its native validation, so "K1
waits for this item" is not offered. What his words leave open is whether
this item waits for K1. Proceeding with: **K1 merges first.** This item's
Plan starts when K1's PR has merged (which implies
`project-config-trust`'s, a8); the item then returns to Design once
(FLOW-06) for the delta K1's structure brings, which **In K1's
structure** lists and which is small, and is implemented once, in the
structure that lasts. The hold is written as a8's is. Its BOOT-02 wording on top of K1's is given under
**Rule changes** and approved by this review, so no rule is reviewed
twice. The cost is stated plainly: "Remove compilation now" becomes
"after K1 has merged". What F9 names, a repository's source compiled on
the host, is closed by `project-config-trust`'s merge either way; what
waits is a `warden` inside its own tree compiling that tree. Confirm.

- (A) K1 first, as above. *Recommended*: no second review of K1, the
  program's key item is not disturbed, no code is written twice, and
  each of the four blocking native hosts validates this item once. It
  is FLOW-05's own reasoning: two features that reshape the same
  structure make each other's consolidation costly, and working them in
  parallel is "the fallback ..., not a goal".
- (B) No order: both are planned when `project-config-trust` has merged,
  and whichever reaches Publish second consolidates on the other
  (FLOW-10). If that is this item, its Implement is redone inside K1's
  structure and its native hosts validate again. If it is K1, K1 goes
  back to Design (FLOW-06, FLOW-14) for its BOOT-02 wording, its named
  fallback "beside the binary, else the source tree" and its
  `relaysource` types, and its native hosts validate again; that delays
  K1 by this item after all. Sooner for this item only if it wins the
  race.
- (C) This item first, and K1's Plan waits for it. Shown for
  completeness: it is the order Jeff's answer to `project-config-trust`'s
  a6 ruled out.

**a2. The minimum, and BOOT-02.** Proceeding with: the minimum is the
version of the `warden` that looks; newer is accepted; `dev` takes only
`dev`; BOOT-02 is amended from "matches the orchestrator's version" to
"at or above the orchestrator's" (**Rule changes**, **Bootstrap
comparison**). The amendment is what makes `alters_relationships` true
(**The front matter, argued**), so this item holds the third of
FLOW-05's three slots beside `driver-contract` and `seam-specs`, and
until one of the three has merged no other item that alters
relationships can be planned, which in the program's second wave means
S2, S3 and S4 (each changes a seam). Under a9 (A) this item holds its
slot while it waits for K1. Confirm.

- (A) as above. *Recommended*: "on or above a minimum version" in his
  words, and the form that survives K3.
- (B) exactly `warden`'s version; a newer file on `PATH` is passed over
  like an older one. BOOT-02 stands as written, no rule change, no
  Bootstrap comparison, `alters_relationships: false`, and the slot is
  free.
- (C) a minimum per artifact, kept in code and lower than `warden`'s
  version. BOOT-02 amended as in (A).

**a3. How an executable states its version.** Proceeding with: one line
of bytes in the file, `buildwarden-version=<version>;`, set when the
binary is linked (by `make build`, to `dev` unless told otherwise, and by
the release build) and found by reading the file. It is the version
`warden --version` prints, and `warden-io` and the relay gain a
`--version` that prints theirs. Confirm.

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

**a4. The container driver's images.** On `main` a development `warden`
builds the relay image per build and the holder and sidecar images by
content hash, so no checkout runs another's. Proceeding with: `make
images CTR=<runtime>` builds the three images, tags each by the hash of
its inputs (`warden-relay-dev:<hash>`, `warden-holder-dev:<hash>`,
`warden-netns-dev:<hash>`) and writes the references to
`dist/warden-dev-images.json`; a `warden` with no reference compiled in
runs exactly those, read from beside itself only, never pulled and never
built by `warden`; when the record or an image is absent the build is
refused before anything is created, with the fix naming `make images`;
`relay_image = "dev"` and `WARDEN_IMAGE=dev` mean the recorded relay
image; a release's defaults are untouched (the relay from
`ghcr.io/buildwarden/relay:latest`, the holder and sidecar by digest); no
version is checked on an image. Confirm.

- (A) as above. *Recommended*: each checkout runs the images it built,
  a rebuilt relay cannot be run from a stale image, and no `warden
  clean`, old or new, removes them.
- (B) three fixed tags, `warden-relay:dev`, `warden-holder:dev`,
  `warden-netns:dev`, and no record. Less to build; one set per runtime
  is then shared by every checkout, `warden clean` must be taught to skip
  `dev` and every older `warden`'s `clean` still removes the relay's, and
  each native container case must run `make images` from the branch under
  test immediately before its builds and record the image IDs.
- (C) as (A), and the value `dev` is removed: a release refuses it with
  the fix, and a development `warden` needs no key to use its images.

The release default being the moving tag `:latest` is as on `main`, and
K1's design notes it; pinning it belongs with seam 8's published hashes
and is not proposed here.

**a5. Boot assets and what a release ships.** Proceeding with: boot-asset
sets (qemu's relay VM and direct-boot kernel and initramfs, Firecracker's
three files) are found by the same lookup, each set from one directory,
and checked by the link-channel stamp where the set has one and the
build boots the stamped file; no release ships them, as today, and the
error says how to build and place them; Hyper-V's relay boot VHDX stays
where it is (the hyperv cache, or `WARDEN_RELAY_BOOT_VHDX`), since
`warden` neither compiles it nor looks in a tree for it; no Windows
release is added. Confirm.

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

**a7. The release's file names, and the release build (Decisions 9 and
12).** No published release has the layout `main`'s configuration
describes: the last one, v0.5.0, holds `warden` and `warden-io`, and the
only release attempted since (v0.6.0) failed in the hook. Proceeding
with: a new archive carries `warden-io-<os>-<arch>` and
`warden-relay-<os>-<arch>` and no `warden-io` or `warden-relay`;
`install.sh` installs every `warden*` file of whatever archive it
downloaded and leaves an older `warden-io` where it is; the error for a
missing executor names an older install's `warden-io` when one is beside
`warden`, until 1.0; the hook is rewritten so that it runs, and CI builds
a snapshot release. Confirm.

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
this design's review. Under a9 (A) the merge named is K1's, which
implies this one. Confirm.

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

### The front matter, argued

`alters_relationships` (FLOW-02) is true when a change alters "a
versioned seam, the bootstrap model (BOOT rules), which component owns a
responsibility (relay, collector, executor, orchestrator, a driver), or
the `driver.Driver` / `driver.Session` contract". Two grounds were
weighed.

- *The bootstrap model.* BOOT-02's sentence changes. That is a ground,
  and the only one this design claims.
- *Which component owns a responsibility.* Compiling the executor and
  the relay stops being something the orchestrator and the drivers do.
  It does not move to another component: it goes to development tooling
  and the release build, which are not among the components FLOW-02
  lists, and choosing which file to deliver stays where it is, in the
  drivers through one shared lookup. No component takes a responsibility
  from another, so this is not a ground.

So a2's option (B), which leaves BOOT-02 as written, would make the value
`false`.

GLOBAL-08 is relied on and not changed: the orchestrator "treats the
relay and executor as fixed, non-substitutable strong dependencies" and
"is not required to support swapping in a different relay or executor".
The lookup locates BuildWarden's relay and executor wherever they are
installed. A different build of either in a `PATH` directory is used when
it passes the check, because Jeff's sentence puts `PATH` first, and it is
recorded; nothing is promised about a build that is not BuildWarden's
own, and the reference says so. ORCH-03 and UX-07 are relied on: the
meaning of the value `dev` of `relay_image` changes and is documented in
the same change, the precedence `flag > warden.toml > ~/.config/warden`
is untouched, and no variable or key is added. `PATH` is not a
configuration level; it is where the host's programs are found, and the
lookup reads it as K1's does.

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
| 2g | `installWardenIO` (`driver/vz/image.go`) | executor baked at `image prepare` and at its refresh (deviation D2) | `warden-io-darwin-arm64` | vz |
| 3 | `goBuildLinux` (`driver/container/images.go`) | `go build` for Linux in the tree | rows 3a to 3c | container |
| 3a | `buildRelayFromSource` (`container.go`) | builds `./cmd/relay`, then an image `warden-relay:<build id>` from an inline Containerfile on `alpine:latest`; removed after the build | relay image | container: version `dev`, or `relay_image = "dev"` on any version |
| 3b | `buildWardenIO` (`container.go`) | a release reads `warden-io` beside `warden`; a `dev` version, or a release that does not find the file, builds `./cmd/warden-io` | executor | container |
| 3c | `buildHolderImage` (`images.go`) | builds `./cmd/warden-pause` and an image `warden-holder:<content hash>` | holder image | container, when no digest is compiled in (any build but goreleaser's) |
| 4 | `buildNetnsImage` (`images.go`) | builds `warden-netns:<content hash>` from an embedded Containerfile; no tree, but an image build | sidecar image | container, when no digest is compiled in |
| 5 | `relayvm.Find` (`driver/relayvm/assets.go`) | All files beside `warden` under release names, else all from `tools/*/output/` in the tree | `relay-vm-vmlinuz`, `relay-vm-initramfs.cpio.gz`; `build-vm-vmlinuz`, `build-vm-initramfs.cpio.gz`; `firecracker-vmlinux`, `firecracker-relay-initramfs.cpio.gz`, `firecracker-build-initramfs.cpio.gz` | qemu, firecracker |
| 6 | `relayvm.StampPath` | The stamp is `link-channel` in the tree's output directory, or `<file>.link-channel` beside `warden` | the stamps of row 5 | qemu, firecracker, direct boot only |

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
- `warden clean` removes images named `warden-relay:<tag>` whose tag is
  not a live build ID (`cmd/warden/clean.go`), the leftovers of row 3a.
  Nothing removes the content-hash images of rows 3c and 4.

**What a release ships.** Three columns of fact: the last published
release, what `main`'s configuration describes and has never produced,
and what a developer has.

| Artifact | Last published release, v0.5.0 (archives downloaded and listed) | `main`'s `.goreleaser.yaml` and `install.sh` (identical at `v0.6.0` and `ad4ea50`; never released) | Looked up as | `make build` |
|---|---|---|---|---|
| `warden` | in every archive (linux, darwin; amd64, arm64) | the same | | `dist/warden` |
| executor, Linux | `warden-io`, installed by `install.sh` | `warden-io`, installed | `warden-io` (container, release); `warden-io-linux-<arch>` (`GuestBinary`) | `dist/warden-io-linux-{amd64,arm64}` |
| relay, Linux | not shipped | `warden-relay`, not installed | `warden-relay-linux-<arch>` | `dist/warden-relay-linux-{amd64,arm64}` |
| executor and relay, macOS arm64 | not shipped | `warden-io-darwin-arm64`, `warden-relay-darwin-arm64`, in every archive, not installed | those names | on macOS arm64 only |
| executor, Windows | not shipped | not shipped | `warden-io-windows-<arch>.exe` | not built; `GuestBinary` builds it on first use |
| boot assets (row 5) | not shipped | not shipped | the release names of row 5 | not built; the scripts write `tools/*/output/` |
| relay image | `ghcr.io/buildwarden/relay:latest` and `:v<version>` (`relay-image.yml`), pulled | the same | `:latest` | built per build |
| holder, sidecar images | by digest, compiled into `warden` (`release.yml`) | the same | the digest | built per build, tagged by content hash |
| Hyper-V relay boot VHDX | not shipped | not shipped | the hyperv cache | `refresh-cache.sh` |

The published releases are v0.1.0, v0.2.0, v0.4.0 and v0.5.0 (the
releases API). The tag `v0.6.0` exists and its release run failed
(run 41251092199, 2026-10-31). `main`'s release configuration has three
faults, in the order a release meets them:

1. **The hook cannot run.** It is written `- sh -c >` followed by the
   commands, and goreleaser runs `sh -c` with no argument. From that
   run's log: `hook failed: command failed cmd=sh output=sh: 0: -c
   requires an argument`.
2. **The hook writes into goreleaser's own `dist/`.** Run for this
   design in a scratch clone with goreleaser v2.20.2 and the hook's form
   corrected: `release failed ... error=dist is not empty, remove it
   before running goreleaser or use the --clean flag`, under `--clean`,
   because the hook runs after the directory is cleaned and before
   goreleaser checks that it is empty. With the hook writing to another,
   git-ignored directory and the darwin targets left out, the snapshot
   release succeeds and its Linux archives hold `warden`, `warden-io`,
   `warden-relay` and the two macOS files.
3. **The macOS arm64 `warden` does not compile.** `GOOS=darwin
   GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/warden` fails at `ad4ea50`
   (`driver/vz/vm.go:25:22: undefined: linuxVMConfig`: the cgo file and
   its stub are both excluded).

`goreleaser check` also exits non-zero on `main`'s file, for two
deprecated properties of `archives`, and it does not see fault 1.

So today an installed release runs the container driver and nothing else
without a source tree, there is no Windows release, and no release can be
cut. Faults 1 and 2 are in the lines this item rewrites and are fixed
with them (Decision 12). Fault 3 is not this item's (**Handed on**); the
macOS rows below say what the archive carries once it builds.

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
| boot assets: qemu direct boot | `build-vm-vmlinuz` + `build-vm-initramfs.cpio.gz`; stamp `build-vm-initramfs.cpio.gz.link-channel` | qemu, builds with no disk image |
| boot assets: firecracker | `firecracker-vmlinux` + `firecracker-relay-initramfs.cpio.gz` + `firecracker-build-initramfs.cpio.gz`; stamp `firecracker-build-initramfs.cpio.gz.link-channel` | firecracker; the stamp, builds with no disk image |

These are the names `GuestBinary` and `relayvm` use today for a file
beside `warden`; none is new. A stamp is read from beside the file it
names and is not one of the files that make a directory a candidate.
K1's convention `warden-driver-<name>` names a program `warden` runs with
commands. An artifact is a file a driver delivers into a guest, boots or
(the relay under vz) starts; it takes no commands from `warden`, so it
keeps `warden-io-` and `warden-relay-` and is not a `warden-driver-`.
The `.exe` is part of the name on every host.

Not in the list, and not found by the lookup: the container driver's
three images (Decision 6), and Hyper-V's relay boot VHDX, whose relay is
compiled into the disk by `refresh-cache.sh`. `cmd/collector` and
`cmd/warden-pause` are built from `cmd/` too; the collector runs inside
`warden`, and `warden-pause` exists only inside the holder image.

### The lookup

For one artifact, resolved once per `warden` invocation to absolute
paths:

1. **`PATH`.** Each entry in `PATH`'s order. The first directory that
   holds a regular file (symbolic links resolved) for every file name of
   the artifact is *the `PATH` candidate*, and the search of `PATH` ends
   there, whichever directory it is. If it passes the artifact's check,
   it is used.
2. **Beside `warden`**, when `PATH` held no candidate or its candidate
   did not pass, and that candidate was not `warden`'s own directory. If
   the files are there and pass, they are used.
3. Otherwise an error that names the artifact, each place, what was
   found there and why it was not used, and the fix (**What stops
   working**).

The details, each the same as K1's lookup unless it says otherwise:

- **A set comes from one directory.** A kernel from one directory is
  never booted with an initramfs from another (`relayvm.Find`'s rule
  today). A directory with some of a set's files is not a candidate.
- **`warden`'s own directory on `PATH`.** It is a candidate like any
  other (`os.SameFile` on the files tells that it is `warden`'s own),
  it is reported as `beside-warden`, and it is looked at once. So a copy
  is used in place of the one beside `warden` only from an absolute
  `PATH` directory that comes *before* `warden`'s own, or from any
  absolute `PATH` directory when `warden`'s own is not on `PATH` (as
  under sudo's reset `PATH`). A copy in an entry after `warden`'s
  directory is never looked at: not when the copy beside `warden`
  passes, and not when it fails, in which case the lookup has one place
  and its error says so. An install nobody has touched reports
  `beside-warden` for everything. This is K1's rule in K1's words.
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
  equivalent. A copy in an earlier `PATH` directory wins when it passes
  the check, and that is the whole of it.
- **Newer is used.** A candidate above the minimum is used with no line
  of its own; the record and doctor show its version.
- **When.** A driver looks up everything the request needs before it
  creates anything: no VM, container, network or work directory exists
  when a lookup fails (UX-04, UX-08). Only what the run uses is looked
  up, and a check that applies only to some builds is applied only to
  them: a Hyper-V build never asks for a Linux executor, and a build
  from a disk image never reads a direct-boot stamp.
- **Never run.** The lookup reads files and directories and runs
  nothing.
- **Read once.** When the build stages a file the lookup found, it reads
  the file once into the build's work directory (or, for the file vz
  bakes, into the image being prepared), taking the SHA-256 and making
  the check again on the bytes it copies. Every later use is of that
  copy, and the digest recorded is the copy's. If the copy does not pass
  the check the file passed in place, the file changed in between, and
  the build stops with an error that names it. A boot file that is only
  read to compose a per-build file (the base initramfs under
  `relayvm.RelayInitrd`) is hashed from the bytes that are composed.

**The check**, by kind:

| Kind | Passes when | Applied |
|---|---|---|
| executor, relay | the file states a version (below) that is at or above `warden`'s; for a `dev` `warden`, exactly `dev` | always |
| boot assets with a stamp (qemu's direct-boot set, firecracker's set) | the stamp beside the stamped file is present and holds the channel this `warden` needs (`driver.LauncherStamp`, as today: `buildlink.Channel`). Missing and stale are the two failing states, as today | only to a build that boots the stamped file (no disk image), as today |
| boot assets with no stamp (qemu's relay VM kernel and base initramfs) | every file is present | always |

**Recorded.** When a build ends, successfully or not, `warden` writes
`<output>/host-files.json` from what the build used:

```json
{"warden": "0.7.0",
 "files": [
   {"name": "warden-io-linux-amd64", "kind": "executor",
    "path": "/home/ana/.local/bin/warden-io-linux-amd64",
    "version": "0.7.0", "sha256": "9f2c...", "found": "beside-warden",
    "passed_over": [{"path": "/home/ana/bin/warden-io-linux-amd64",
                     "found": "PATH", "reason": "is 0.6.0, below 0.7.0"}]}],
 "images": [
   {"role": "relay", "reference": "ghcr.io/buildwarden/relay:latest",
    "id": "sha256:41c0...", "source": "default"}]}
```

- `files`: one entry per file the lookup found. `found` is `PATH` or
  `beside-warden`. A boot-asset file carries the stamp's value in place
  of `version` when the build checked one. `sha256` is the staged
  copy's.
- vz's executor is not looked up by a build (D2). `image prepare`, and
  its refresh, look it up, print one line naming the file they install,
  and write its name, version, digest and the time into the prepared
  image's directory, where the image's provenance already is
  (`recordPreparedImage`). A vz build copies that into `files` with
  `found: "prepared-image"`. An image prepared before this item has no
  such record and its entry says `version: "unknown"`.
- Hyper-V's relay boot disk is an entry of kind `relay-boot-disk` with
  its path, its stamp and `found: "hyperv-cache"` or
  `"WARDEN_RELAY_BOOT_VHDX"`. No digest is taken: the disk is the
  parent of a per-build differencing disk, attached in place.
- `images` (container driver): the relay, holder and sidecar, each with
  the reference used, the image ID the runtime resolved it to before the
  container was created, and `source`: `default`, `relay_image` with
  where it was set, `compiled-in digest`, or `development record`.

The file is unsigned and not part of the ledger, like K1's `driver.json`;
it is the machine-readable form (UX-02). `warden doctor --driver <name>`
prints the same for everything the driver can need, in a block named
`Host files`: each file with where it was found and its version or
stamp, each image with whether it is in the runtime's store, and for a
`warden` that needs the development record, whether the record is there
and each image it names is present, with the `make images` fix. Doctor
fails on what every build of the driver needs and cannot be used. Under
`sudo` (Firecracker's doctor is run that way) the block shows the lookup
as that process makes it and says which `PATH` it searched, since a
build run without `sudo` searches the user's.

### The version

**What states it.** The three programs that are looked up or do the
looking, `warden`, `warden-io` and `relay`, link one small package whose
only content is one string variable with no initial value. The build
tooling sets it when it links the binary (`-X`), `make build` to `dev`
unless `VERSION` is given and the release build to the release's version:

```
buildwarden-version=0.7.0;
```

`warden --version` prints the part between `=` and `;`, and that
replaces `main.version`, so a binary has one version. `warden-io` and
`relay` gain a `--version` that prints theirs and exits. That is not for
`warden`'s use, which never runs them to ask: a variable no reachable
code reads is dropped by the linker, and the binary then holds no line.
`warden` reads an artifact's version by scanning the file for
`buildwarden-version=` followed by 1 to 64 characters of
`[0-9A-Za-z.+-]` and a `;`. The string is ordinary read-only data in an
ELF, PE or Mach-O file, so the scan is the same for every OS and
architecture and for a binary in any language.

What was tried, in a scratch module with go1.24.7, for linux/amd64,
windows/arm64 and darwin/arm64 (and by the two reviewers of this design
in this repository's own programs: `cmd/relay` and `cmd/warden-io` for
five OS and architecture pairs, `cmd/warden` for three):

| Build | Lines found in the binary |
|---|---|
| the package only imported, the variable never read; `-trimpath -ldflags "-s -w -X ..."` | 0 |
| the variable printed by a `--version` branch; `-trimpath` | 1 |
| the same without `-trimpath` (the Makefile's and the hook's flags today) | 3, identical: Go's build information records the `-ldflags` |
| a bare `go build` | 0 |

The design follows each row. Every program reads the variable on a
reachable path. Every build in the Makefile and the release build uses
`-trimpath`. The reader takes every match in the file: when all are
identical, that is the version; a file with no match, or with two
matches that differ, states no version and does not pass. So a binary
built without `-trimpath` still states one version, and a binary built
with a bare `go build` states none; a `warden` built that way calls
itself `dev`. Two more things the first trial showed stand: the variable
must have no initial value (with one, the binary holds the initial line
and the set one, which differ), and the package that holds the variable
must not spell the prefix itself, so it lives apart from the scanner.
The guard is CI's check on the real `dist/` binaries (**Development
tooling**).

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
executor someone left on `PATH`. `make build VERSION=0.7.0` builds
`dist/warden` and every executable beside it with that version, so the
set stays consistent; such a `warden` has no image digest compiled in
and takes its holder and sidecar from the development record (**The
relay image**).

**What this does not detect.** Two development builds are both `dev`, so
a `dist/warden-io-linux-amd64` left from an older commit beside a freshly
built `dist/warden` is used. On `main` the cache rebuilt it when `warden`
was newer. Keeping `dist/` whole is `make build`'s job now, which is what
"purely development processes" asks.

**BOOT-02.** On `main`: "delivered per build and matches the
orchestrator's version". After this item the executor is at or above the
orchestrator's version, and newer when a newer one is in an earlier
`PATH` directory. That is a change to an invariant's wording, so it is a
rule change with a Bootstrap comparison (BOOT-07). How an executor states
its version, and what `dev` means, are added to `bootstrap.md`'s model
text, so the rule rests on terms the same file defines (**Rule
changes**). Deviation D2 (vz bakes the executor at prepare) is unchanged:
the file baked is the one the lookup found at prepare time, the check is
made then and not per build, and the image carries the record of it.

### The relay image, the holder and the sidecar

Which image a build uses, keyed as `main` keys it: the holder and the
sidecar on whether a digest is compiled in, the relay on `relay_image`
and then on the version.

| Image | Used |
|---|---|
| relay | `relay_image` / `WARDEN_IMAGE` set to a reference: pulled, as today. Set to `dev`: the relay image of the development record. Unset: version `dev` takes the development record's; any other version pulls `ghcr.io/buildwarden/relay:latest`, as today |
| holder | the digest compiled in (goreleaser sets it), pulled when absent, as today; with none compiled in, the development record's |
| sidecar | the same as the holder |

**The development record** is `warden-dev-images.json` beside `warden`,
written by `make images`:

```json
{"relay":   "warden-relay-dev:3f9a1c0e52b7d4aa",
 "holder":  "warden-holder-dev:8d02c41b7e6f9a13",
 "sidecar": "warden-netns-dev:c7a94e10b3f25d68"}
```

Each tag is the first 16 hexadecimal digits of the SHA-256 of the
image's inputs, its Containerfile and the files of its build context,
which is how `main` tags the holder and the sidecar. The same inputs give
the same tag in any runtime, so one record serves `finch`, `docker` and
`podman` alike, and two checkouts at the same commit share an image
while two that differ do not. `warden` reads the record from its own
directory only: it is not an artifact and is never looked for on `PATH`,
because it stands in for the references a release has compiled in. An
image the record names is looked for with `<runtime> image inspect` and
is never pulled and never built by `warden`. When the record is absent,
or an image it names is not in the runtime's store, the build stops
before anything is created (**What stops working**, 1).

`warden` no longer embeds the two Containerfiles, removes no relay image
after a build, and tags nothing. `warden clean`'s sweep of
`warden-relay:<build id>` leftovers stays as it is: the development
images' repository names end in `-dev`, which that sweep does not match,
in this `warden` or in any older one. `make images` removes the images
its previous record named when their tags have changed, so a checkout
does not pile them up. `warden clean --cache`, which already removes
what earlier mechanisms left, also removes `main`'s leftover
`warden-holder:<16 hex>` and `warden-netns:<16 hex>` images, which
nothing removes today; it is the explicit form, because an older
development `warden` still in use rebuilds them on its next build.

The executor copied into the build container is `warden-io-linux-<host
arch>` from the lookup, for a release and a development build alike.

### Development tooling

- **`make build`** builds, with `-trimpath` and the version string,
  `dist/warden` and beside it: the Linux executor and relay for amd64
  and arm64 (as today), the Windows executor for amd64 and arm64 (new:
  Hyper-V and qemu Windows guests asked `GuestBinary` to build it), and
  on macOS arm64 the two macOS binaries (as today). It then copies each
  boot asset the tree has built from `tools/*/output/` to `dist/` under
  its release name, with its stamp as `<name>.link-channel`. An asset
  that has not been built is not copied and not an error. `VERSION` is
  `dev` unless given.
- **`make artifacts OUT=<dir>`** is the part of `make build` that builds
  the executables other than `warden`, for every OS and architecture in
  the artifact list. `make build` runs it with `OUT=dist`; the release
  hook runs it with another directory (**Releases and install**). One
  recipe, so a development build and a release build cannot drift.
- **`make images CTR=<runtime>`** (new) builds the three development
  images for the host's architecture in that runtime's store and writes
  the record. The relay's is built from `Dockerfile.relay`, the file the
  published image is built from, in place of `main`'s inline
  Containerfile on `alpine:latest`; the target stages `relay-<arch>`
  from `dist/warden-relay-linux-<arch>` and passes `--build-arg
  TARGETARCH=<arch>`, as `images.go` does for the holder today.
  `Dockerfile.relay` gains `COPY --chmod=0755`, as `holder.Containerfile`
  has: `main`'s inline Containerfile sets the mode in the image because
  "a context on a Windows host carries no exec bit", and
  `Dockerfile.relay` does not. The published image is unchanged by that
  (its relay is built 0755 on a Linux runner). The holder's context is a
  `warden-pause-<arch>` the target builds, and the sidecar has none.
  `CTR` defaults to the first of `finch`, `docker`, `podman` whose
  `info` succeeds, which is what `warden` detects
  (`DetectRuntime`), not the first on `PATH`. It is separate from `make
  build` because it needs a running container runtime.
- **The asset scripts** keep writing `tools/*/output/`. Their closing
  lines and `warden`'s fix messages say to run `make build` afterwards.
- **`go run ./cmd/warden`** runs a `warden` in a temporary directory with
  nothing beside it. It finds artifacts only on `PATH`. It is no longer a
  way to run a build from a checkout; `dist/warden` is, as
  `CONTRIBUTING.md`, CI and the native sessions already do. A **`go
  install`**ed development `warden` works when `dist/` is on `PATH` or
  the artifacts are installed beside it, and for the container driver
  when the record is copied beside it.
- **Tests.** Six tests reach the code the lookup replaces (found by the
  review, with `ModuleRoot` made to fail and to log its callers, on
  Linux). Four cross-build `warden-io` through `GuestBinary`:
  `TestStageSeedDir`, `TestStageSeedDirConfiguredLink`
  (`driver/hyperv/winseed_test.go`), `TestWindowsSeedDirRunScript`
  (`driver/qemu/windows_test.go`) and `TestDescribeBuildEvalImage`
  (`driver/hyperv/environment_test.go`); each only needs a file to stage.
  Two reach `relayvm.Find` through `firecracker.findAssets`:
  `TestDoctorReport` and `TestDoctorUnderSudo`
  (`driver/firecracker/preflight_test.go`). No test reads the process's
  `PATH` or `warden`'s directory, or its result would depend on what the
  machine has installed: `driver` gains one documented, test-only
  function that points the lookup at a directory of the test's own and
  accepts what is there, every test that reaches the lookup calls it,
  and the tests write placeholder files. It replaces
  `project-config-trust`'s test-only setter of the tree and its two
  `TestMain`s. No test compiles anything, and `make test` needs no
  `dist/`.
- **CI.** The `build` job of `main.yml` runs `make build` and then
  checks that every executable in `dist/` states the version
  `dist/warden` does, once; its Linux leg also builds the snapshot
  release (**Releases and install**). `test-integration.sh` runs `make
  build images`, so the `integration` workflow exercises the development
  images on docker. The `firecracker` workflow already runs the asset
  scripts and then `make build`, which now places the assets; its cache
  of `tools/*/output/` is unchanged. Both workflows' `pull_request.paths`
  gain `Makefile`, which now carries behaviour they depend on and is in
  neither list today.
- **Native sessions** build with `make build` in their checkout, as
  today; a session that validates the container driver also runs `make
  images`. The cases that need `PATH` changed for `dist/warden` (a
  recording `go` first, a copy in a `PATH` directory, a `.` entry) are
  one tracked script, `tools/native/host-files-cases.sh`, because the
  unattended session's launcher passes only `WARDEN_*` assignments
  (`tools/workflow/native/native-run.sh`) and runs a branch's script
  under `tools/` as it is. The skill is not edited by this item
  (FLOW-14): its text says `make build`, which stays true, and the queue
  steps name `make images` and the script.
- **Leftovers.** The per-tree cache `CacheDir()/bin/` is no longer read
  or written. `warden clean --cache` removes it, as it removed the flat
  cache before.

### Releases and install

- **`.goreleaser.yaml`.** The hook becomes one command that can run:
  `make artifacts OUT=release-artifacts VERSION={{ .Version }}`. The
  directory is not goreleaser's `dist/` and is git-ignored (faults 1 and
  2 of the **Inventory**). Each archive carries `warden` and, under the
  names of **The artifacts**: the Linux executor and relay and the
  Windows executor for the archive's architecture, and in the macOS arm64
  archive the two macOS binaries as well. The files `warden-io` and
  `warden-relay` are no longer in it, and the macOS binaries are no
  longer in the Linux archives. `archives` moves off its two deprecated
  properties, so `goreleaser check` passes.
- **`install.sh`** is served from `main`
  (`curl .../main/install.sh | sh`, in `README.md` and the installation
  guide) and installs the latest published release, or the version given
  as `$1`. From the day this item merges it runs against v0.5.0's
  archive, for as long as no newer release exists, and for ever for
  `install.sh v0.5.0`. It therefore names no file but `warden`: it fails,
  before moving anything, when the archive holds no `warden`, and then
  installs every regular file of the archive whose name begins with
  `warden` into `INSTALL_DIR`. An old archive installs `warden` and
  `warden-io`, as today; a new one installs every artifact. A
  `warden-io` already in `INSTALL_DIR` from an older install is left
  alone.
- **What an installed release then runs with nothing built:** the
  container driver, as today. For qemu and Firecracker it has the relay
  and the executor; the boot assets are still to be built (a5). vz has
  everything once a macOS arm64 archive can be built. Hyper-V has no
  release.
- **Proven by running it.** The Linux leg of the `build` job runs
  `goreleaser check` and `goreleaser release --snapshot --clean
  --skip=publish,sign`, lists each archive it made against the artifact
  list in the code, and reads each archived executable's version line.
  A full snapshot builds the macOS arm64 `warden`, so it passes only
  once fault 3 is fixed. If that fix has not merged when this item
  reaches Review, the step runs on a copy of the configuration with the
  darwin targets left out, which is what was run for this design, and
  the fix item removes the reduction; until then the macOS archive's
  layout is unproven, and this design says so.
- **Tests.** A tier-0 test reads the artifact list from the code and
  checks that `make artifacts` names every executable in it. A second
  runs `install.sh`'s install step, which is a function for the purpose,
  against a directory in each layout: `warden` and `warden-io` (v0.5.0),
  the new layout, and one with no `warden`.

### What stops working, and what it says

Against `main` at `ad4ea50`. `project-config-trust` already ends cases 5
and 6 for a `warden` outside its tree, with messages this item replaces.
Every message is a `driver.FixableError` (UX-04) raised before anything
is created.

1. **A `warden` with no image digest compiled in (any `make build`),
   container driver, no `make images`.** On `main` the images are built
   per build.

   ```
   ERROR: this warden has no development images recorded beside it (/src/buildwarden/dist/warden-dev-images.json not found)
     To fix: build them in warden's source tree: make images CTR=docker
   ```

   When the record is there and the runtime does not have an image it
   names (built for another runtime, or removed):

   ```
   ERROR: relay image warden-relay-dev:3f9a1c0e52b7d4aa, recorded by make images, is not among docker's local images
     To fix: build the development images for docker in warden's source tree: make images CTR=docker
   ```

   The same for the holder and the sidecar. It is also what a rebuilt
   relay says until `make images` has run again, by design.

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
   direct-boot set's messages name their own files and scripts. A
   direct-boot build whose stamp is missing or stale keeps today's
   sentence for qemu and Firecracker, "<asset> predates the configurable
   build link and must be rebuilt once (<what the stamp holds>)", with
   the rebuild script and `make build` as the fix; a stamp that fails in
   a `PATH` directory is passed over for the copy beside `warden` first.

4. **A `warden` rebuilt alone, beside older executables of another
   version.** On `main` the cache rebuilt them. Two development builds
   are both `dev` and are not told apart (**The version**); a release
   beside older files says:

   ```
   ERROR: executor warden-io-linux-amd64 is too old for this warden (0.7.0)
            /home/ana/bin/warden-io-linux-amd64 is 0.6.0 (PATH)
            /home/ana/.local/bin/warden-io-linux-amd64 is 0.6.2 (beside warden)
     To fix: install this release's files beside warden (re-run install.sh, or copy every file of the release archive), or put warden-io-linux-amd64 0.7.0 or later in a PATH directory before warden's
   ```

   When `warden`'s own directory is the first on `PATH` to hold the
   file, there is one place and one line: `/home/ana/.local/bin/warden-io-linux-amd64
   is 0.6.2 (beside warden, which is also the first PATH directory that
   holds it; later PATH directories are not searched)`. A file that
   states no version reads `states no version`; a release file met by a
   development `warden` reads `is 0.6.0, and a development warden uses
   only development builds`, with the fix of case 2.

5. **`go run ./cmd/warden build`, and a `go install`ed development
   `warden`**, in a checkout. On `main` the working directory's tree is
   built. Now the message of case 2, unless the artifacts are on `PATH`.

6. **An installed release used inside a buildwarden checkout with a VM
   driver or `relay_image = "dev"`.** On `main` this compiles the
   checkout: the hole. Now a VM driver uses the release's own executables
   and gets case 3 for the boot assets, and `relay_image = "dev"` gets:

   ```
   ERROR: relay_image = "dev" (./warden.toml) names the development relay image, and this warden has none recorded beside it (/usr/local/bin/warden-dev-images.json not found)
     To fix: remove relay_image to use this release's relay, or run a warden built in a buildwarden source tree (make build images)
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

9. **A file that changes between the lookup and its staging** (new; on
   `main` nothing is checked).

   ```
   ERROR: /home/ana/bin/warden-io-linux-amd64 changed while this build was starting (it stated 0.7.0 when found and states no version as copied)
     To fix: run the build again once the file is no longer being replaced
   ```

Looked at and found not to stop:

- **`install.sh` from `main` against a published archive.** It installs
  whatever `warden*` files the archive holds, so v0.5.0 installs as it
  does today.
- **A Firecracker or qemu build from a disk image with a missing or
  stale direct-boot stamp.** The stamp is not read for such a build, as
  on `main`.
- **`make build VERSION=<v>`.** `dist/` is one version throughout; the
  relay image is pulled as for any non-`dev` version, as on `main`; the
  holder and sidecar need `make images` (case 1), where `main` built
  them.
- **`warden clean`, of any version**, leaves the development images.
- **A qemu direct-boot initramfs built by `main`'s script**, which still
  carries a baked executor: the executor appended per build replaces it.
- **A vz image prepared before this item**: builds run; the record says
  its executor's version is unknown until the next prepare or refresh.

Nothing else stops: a release installed by `install.sh` runs the
container driver as before, and a tree built with `make build` (and `make
images` for the container driver) runs every driver it ran.

### Reference docs (UX-01) and the overview (FLOW-15)

- `docs/book/src/guide/configuration.md`: the `relay_image` comment and
  the **Relay Image** section (`dev` is the development image recorded
  beside `warden`; a development build uses it by default; nothing is
  compiled).
- `docs/book/src/guide/drivers.md`: a new section, **Host files**: the
  list, the lookup, the version, the record, what a file in an earlier
  `PATH` directory does and how to see it, and that BuildWarden supports
  only its own relay and executor; the Firecracker **Boot assets**
  paragraph and the Hyper-V notes say `make build` places the assets.
- `docs/book/src/guide/output.md`: `host-files.json` in the layout and a
  section for it. The page says the layout is "identical for every
  driver"; the file's contents differ by driver and the section says how.
- `docs/book/src/guide/cli.md`: `warden doctor`'s `Host files` block;
  `warden clean --cache`.
- `docs/book/src/getting-started/installation.md`: **One-liner Install**
  and **From Releases** (every file of the archive, and `~/.local/bin`,
  which is where `install.sh` installs; the page says `/usr/local/bin`
  and to move `warden` alone, which leaves the container driver without
  its executor today); **From Source** (the table of `dist/`, `make
  images`); the first-time QEMU, Firecracker and Hyper-V sections.
- `README.md` (the `relay_image` comment) and `CONTRIBUTING.md` ("Local
  builds report `version = "dev"`, which builds the relay from source"
  becomes `make build images`; the release section names the snapshot
  step).
- `docs/book/src/contributing/structure.md`: the new package.
- `docs/book/src/concepts/overview.md` gains, under the orchestrator:

  ```
  find(artifact):                      # an executor, a relay, a boot-asset set
      for dir in PATH, in order:       # warden's own directory included
          dir holds every file of the artifact:
              reached through a relative entry -> error
              check passes -> use it
              else         -> remember why; stop searching PATH
      beside warden, if not already looked at, holds them and the check passes
                           -> use it (say what was passed over)
      else -> error: what was looked for, where, what was found, the fix

  check: executor, relay -> the version the file states >= warden's (dev: only dev)
         boot assets     -> all files present; the stamp, for a build that boots the stamped file
  stage: read the file once into the work directory; check and hash that copy; use only it
  ```

  and the container driver's lines say the development images come from
  `make images` and its record.

### How it sits with the other items

**Order of merges** (a9, option (A)): `runtime-cli-project-path`, then
`project-config-trust`, then `driver-contract` (K1), then this item.

**`project-config-trust`** (read at `0bda630`). This item deletes what it
adds for the tree: `ModuleRoot` with its fixable error, the test-only
setter of the tree, the two `main_test.go` files' use of it, its two
messages that say to run `make build` and use `dist/warden`, and its
sentences in `CONTRIBUTING.md`, the installation guide and the Relay
Image section. Its `stageTestSeed` that fails where it skipped stays. Its
tier-1 case and corpus entry for F9 ("a `warden` outside any source tree,
run in a repository that declares the module, runs no `go`") must pass on
this branch, and this item widens the property: no `warden`, inside a
tree or not, runs `go`. How this item is held behind it is a8.

**`runtime-cli-project-path`** (approved at `a2afd08`; in Publish).
Nothing shared but the convention: errors at once, transitional
detection under a marker until 1.0. UX-08 as it amends it is what
**When** in the lookup follows.

**`driver-contract` (K1)** (approved at `a6c8f0b`; Plan held until
`project-config-trust` has merged). Its design describes `GuestBinary`
and `relayvm.Find` as they are on `main` and wraps them
(`relaysource.Bundled`, with a `Source` path in the tree and a `GoBuild`;
its named fallback "Kept, existing: beside the binary, else the source
tree"), and it amends the sentence of BOOT-02 this item amends. Jeff's
answer to `project-config-trust`'s a6 is that K1 is not held behind this
item. The two orders that leaves, and what each costs under FLOW-14,
FLOW-06 and FLOW-10:

- *K1 first* (a9 (A), proceeding). K1 is planned and built as approved.
  This item's Plan waits for K1's merge and the item returns to Design
  once for the delta below. Its BOOT-02 wording on K1's sentence is
  already what this review approves (**Rule changes**, the second
  setting), so no rule is reviewed twice, and the delta review covers
  only where the code lives. Compilation stays, for a `warden` inside
  its tree, until then.
- *This item first, or a race* (a9 (B)). K1's approval of BOOT-02 was
  "as worded there", on `main`'s sentence. With this item's sentence on
  `main`, K1's wording no longer fits, and "a rule change found after
  the review is not cleared: the feature goes back to Design" (FLOW-14).
  K1 would return for that sentence, for its named fallback, which this
  item removes, and for `relaysource.Bundled` without `Source` and
  `GoBuild`; and whichever item is second repeats its native validation
  on the consolidated branch.

**In K1's structure**, which is where this item is implemented under a9
(A). Expected content of the return to Design; none of it changes a
decision above.

- *Who looks.* The lookup, the version reader and the record are one
  kit package under `driver/`. A driver binary links it, so for
  `warden-driver-container` "beside `warden`" is beside the driver, as
  K1 already says of "the executable", and the minimum is the driver's
  version, which is `warden`'s while they are built together. The
  drivers still linked into `warden` are unchanged by that.
- *`relaysource`.* `Bundled` takes its files from the lookup and loses
  `Source` and `GoBuild`; K1's named fallback is replaced by this
  design's; `Relay.Version` is the version the file states, where K1 has
  `"dev"` for a source-tree build.
- *The record.* K1's contract lets a driver write `relay.log` and
  `console.log` into the output directory "and nothing else", and
  `warden` writes `driver.json`. A driver binary therefore reports what
  it resolved to `warden`, which writes `host-files.json`: an optional
  list on the two messages the driver already sends after resolving its
  relay and after delivering the executor (`relay-started`,
  `environment-started`). That is a change to the driver contract, with
  its specification and suite in the same change (GLOBAL-08 as K1 amends
  it), and it is why the return adds `driver-contract` to this item's
  `seams`. Doctor's `Host files` block is printed by the driver's own
  `doctor` command under K1.
- *The development record and `clean`.* The record is read beside the
  container driver's binary. `warden clean`'s image sweep is the
  container driver's `clean` under K1.
- *BOOT-02.* The second setting under **Rule changes**.

**K2** makes the VM drivers binaries; they keep the lookup as K1's
container driver has it. **K3 and K4** give each driver its own
repository and version: "at or above the driver's own version" stops
meaning anything for the kit's executor, and the driver then states a
minimum for it in code (a2's option (C), at that time). **Track M and
seam 8** (relay release forms with published hashes): each "adopt relay
version X" item (M6) replaces `warden-relay-*` and the relay's boot
assets for one driver with a relay release, and how a relay states its
version becomes seam 8's to say; the version line here is one any
language can carry. **`seam-specs`** (approved at `64a4a95`): its `make
conformance` builds the relay and the collector it tests into
`dist/conformance/`, which is development tooling and no part of what
`warden` looks up; both items add targets to the `Makefile`, in
different lines.

### Handed on

Findings that are not this item's. This agent edits only its own queue
item, so the session that receives this design files them.

| What | Where it goes |
|---|---|
| goreleaser's macOS arm64 build of `warden` (`CGO_ENABLED=0`) does not compile at `ad4ea50`, so no release can be cut once the hook runs; K1's design lists it as "to be filed" and nothing in the queue or backlog carries it | a fix item through intake now, as this item's follow-up: a regression `main` already has (FLOW-09), exempt from the freeze |
| The order with K1 (a9), and what K1 inherits or returns for under each answer | a note on `driver-contract`'s queue item when a9 is answered |
| The release default relay image is the moving tag `:latest` (a4); the relay inside the published image states no version (`relay-image.yml` builds it with a bare `go build`), which nothing checks because images are not version-checked | seam 8 / M6 |
| Boot assets in a release (a5, option (B)) | track M for the relay's, K3 and K4 for a driver's |
| The corpus entries of the Security note | `security-suite` (SEC-03), at Publish |
| Removing `TRANSITIONAL(artifact-names)` at 1.0 | its own small item, filed at Publish |

## Defaults and overrides

| Default or fallback | Why it is the obvious one | What overrides it |
|---|---|---|
| **New, named fallback: `PATH`, then beside `warden`.** Triggered when `PATH` holds no candidate, or its first candidate does not pass the check and is not `warden`'s own directory | Jeff's sentence. A file on `PATH` is where a user puts programs; the copy beside `warden` is the one it shipped with | Nothing names a file. A copy in a `PATH` directory before `warden`'s that passes wins. Seen as: the passed-over line on stderr on every run, `host-files.json`, doctor |
| **New: the minimum is `warden`'s own version**; `dev` takes only `dev` | Everything is built from one tree and released together; no number is kept by hand | None |
| **Changed: with no digest compiled in, the holder and sidecar images** are the ones the development record names; with version `dev`, the relay image too | They are what `make images` built from this checkout; `warden` no longer builds any | The relay: `relay_image` / `WARDEN_IMAGE`, used exclusively. The other two: none |
| **Changed: `relay_image = "dev"`** is the development record's relay image, not a build | The value keeps its name and its intent, "my development relay" | Any other value |
| **Kept: a release's images** (the relay from `:latest`, the holder and sidecar by digest) | As on `main` | `relay_image` for the relay |
| **Kept: Hyper-V's relay boot VHDX** in the hyperv cache | Not compiled by `warden` and not looked for in a tree | `WARDEN_RELAY_BOOT_VHDX` |
| **Removed fallbacks:** the build from a source tree when a file is not beside `warden`; the tree's `tools/*/output/` when assets are not beside `warden`; the build of `warden-io` when a release does not find it; the build of the holder and sidecar images when no digest is compiled in | Each ran only when the first place failed, and each compiled or trusted a tree (Decision 1) | |

No merge is added. A provided `relay_image` that cannot be used is an
error that names it; the default is not tried (GLOBAL-10).

## Cross-driver impact

| Driver | What changes |
|---|---|
| container | The executor comes from the lookup (`warden-io-linux-<arch>`) for every version. A `warden` with no digest compiled in runs the images its development record names and builds none; `relay_image = "dev"` likewise. `goBuildLinux`, `buildRelayFromSource`, the embedded Containerfiles and the content-hash tags go. A release's pulls are unchanged. `warden clean` keeps its sweep, which does not match the development images; `clean --cache` removes `main`'s leftover content-hash images. The three image IDs are recorded |
| qemu | The relay binary, the relay VM set, the direct-boot set and the guest's executor come from the lookup, all before anything boots. Direct boot appends the executor per build (Decision 8). The direct-boot stamp is checked as today, on direct boot only |
| firecracker | The relay binary, the three-file set and the executor come from the lookup. One set, one directory, as its single `relayvm.Find` has it today; the stamp is checked on direct boot only, as today |
| vz | The relay it runs on the host and the executor `image prepare` installs come from the lookup, and prepare records the executor with the image. Under `sudo` the lookup searches the `PATH` that process has |
| hyperv | The Windows executor on the seed comes from the lookup. The relay boot VHDX and its stamp are untouched; its path and stamp are recorded |

No driver is unaffected. The lookup, the version reader and the recorder
are one implementation in `driver/`, used by all five (ORCH-01).

## Specification changes

| Seam | Specification | Change |
|---|---|---|
| `bootstrap-model` | No written specification. It is defined by `docs/development/bootstrap.md` (the diagram, BOOT-01 to BOOT-06, the mapping and the register) | BOOT-02's second sentence, the diagram's "version-matched" label and one paragraph of the model text, as in **Rule changes**, made in that file in Implement, before the code (LOOP-02). The qemu / Linux row of the mapping gains its direct-boot delivery, a correction to match the code as Decision 8 leaves it |

No specification under `specs/` is touched: the ledger and its metadata
schemas do not record the executor or the relay binary, before or after.
Under a9 (A) the return to Design adds the `driver-contract` seam for
the record's message (**In K1's structure**).

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
| BOOT-02 the executor is the only out-of-band input | "It is delivered per build and matches the orchestrator's version." | "It is delivered per build, and its version is at or above the orchestrator's: an executor that states an older version, or none, is not delivered." | container, qemu, hyperv, firecracker: the executor is the file the lookup found, the copy shipped with `warden` unless a copy at or above its version is in an earlier `PATH` directory. qemu direct boot: delivered per build, where it was the asset's. vz: D2 as before; the baked file is the one the lookup found at prepare, the check is made then and not per build, and the image carries the record of it |
| BOOT-03 one start, then hands off | As written | Unchanged | None |
| BOOT-04 build inputs come from the relay | As written | Unchanged | None |
| BOOT-05 setup is the executor's domain | As written | Unchanged | None |
| BOOT-06 images carry hooks, not content | As written | Unchanged. qemu's direct-boot initramfs stops carrying an executor tied to a `warden` version | qemu: the asset no longer needs rebuilding when `warden-io` changes |

### Gains

- The rule says what `warden` enforces. On `main` nothing checks that a
  release's `warden-io` beside it is its own; an upgrade that replaces
  only `warden` runs the old executor without a word.
- A release's executor can be installed anywhere on `PATH` and need not
  sit beside `warden`, and a newer release's executor is accepted, which
  is what a driver in its own repository will need (K3).
- The source of the executor is a file with a version and a digest in the
  build's output, not a compile whose input was whatever tree was found.

### Costs

- **Security posture.** The executor is the one input outside the
  relay, and it is now taken from `PATH` first. Whoever can write a
  directory on the user's `PATH` before `warden`'s can supply it. The
  Security note states this in the terms accepted for K1.
- **The record of what the build consumed.** Unchanged in the ledger:
  the executor was not recorded there and is not now.
  `host-files.json` adds its path, version and digest, unsigned.
- **A newer executor than `warden` can run.** Before 1.0 the relay guest
  API and the `initialize` contract may still move, so a much newer
  executor may not work with an older `warden`'s relay. `warden` does not
  refuse it; the build's failure and `host-files.json` show the versions.
  GLOBAL-08 stands: a relay or executor that is not BuildWarden's own
  build is not supported, and the lookup does not make it so.

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

**Added to the model text**, after the paragraph that begins "Only the
executor crosses into the environment outside the relay", so that the
rule's terms are defined in the file that holds the rule:

> An executor states its version in its own file, as the line
> `buildwarden-version=<version>;`, which the host reads without running the
> file. Versions are ordered as Semantic Versioning 2.0.0 orders them. A
> development build states `dev`, which has no place in that order: a development
> build of the orchestrator delivers only a `dev` executor, and a release never
> delivers one.

**Severity and check:** MUST, `agent-judgment`, before and after. No
enforcement change: `tools/tier1/` is not edited. The `bootstrap-surface`
check sees the edit to `bootstrap.md` beside a design whose `rules` cites
BOOT-07.

**The second setting: K1's sentence on `main`**, which is the one
Implement meets under a9 (A). K1 amends the same sentence, and its
wording was approved by name on 2026-11-15 ("rule change BOOT-02
approved at `a6c8f0b`, as worded there, with the Bootstrap comparison"):
"It is delivered per build by the driver and matches the version of the
driver that delivers it; a driver linked into `warden` has `warden`'s
version." On that sentence the proposed wording is:

> The host delivers exactly one thing to the build environment outside the relay:
> the executor (`warden-io`) and the minimal launcher that starts it. It is
> delivered per build by the driver, and its version is at or above that of the
> driver that delivers it: an executor that states an older version, or none, is
> not delivered; a driver linked into `warden` has `warden`'s version. The build
> script, context files, the CA and extension material never travel this way.

The diagram's label, which K1 makes `per build, the driver's version`,
becomes `per build, at or above the driver's version`, and in the added
paragraph "a development build of the orchestrator" reads "a development
build of the driver". The review is asked to approve the change in both
settings, as K1's review was for ORCH-03. If `main` then holds neither
sentence as quoted, nothing of BOOT-02 is edited and the item returns to
Design (FLOW-06). If this item were to merge first (a9 (B)), K1's
approved wording no longer fits the sentence on `main`, and whether K1's
Implement may write the sentence above on this review's approval, or K1
returns to Design for it, is K1's review's to say; this design does not
clear K1's edit.

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
  user's own image store and be named by a record beside `warden`.
- **Opened by decision: the files are taken from `PATH` first.** This
  is K1's posture, in Jeff's words for it: the lookup "is never outside
  of the user's control (i.e. never a part of the build environment)".
  Nothing inside the build environment can reach `PATH`, `warden`'s
  directory or the host's image store. Whoever can write a directory on
  the user's `PATH` can already replace `git`, the container runtime or
  `warden`, so the lookup gives such a writer no new way in; it gives
  them more files to replace.
- **What a planted file can do.** A file of an artifact's name in an
  absolute `PATH` directory that comes before `warden`'s own (in any,
  when `warden`'s directory is not on `PATH`, as under a reset `PATH`),
  and that passes the check, is used in place of the copy beside
  `warden`. A copy in an entry after `warden`'s directory is not used.
  A planted executor runs as the first process of the build environment
  and decides what the build does; a planted relay carries all of the
  build's traffic and signs the ledger, and under vz runs on the host as
  the user; a planted kernel or initramfs is the relay VM. No check
  stops this, and none is meant to. The version line is a few bytes
  anyone can write; a link-channel stamp is a small file anyone can
  write; and qemu's relay VM kernel and base initramfs have no check at
  all beyond being complete. Each check says that a file fits this
  `warden`, never that it is trusted.
- **What a build context can and cannot do.** It cannot make `warden`
  compile anything. It cannot name an artifact's file: no key, flag or
  variable does. A file it ships is not used from the working directory,
  nor through a relative or empty `PATH` entry, where the lookup stops
  with an error. It can supply an artifact only where the user's `PATH`
  holds an absolute entry inside the repository before `warden`'s
  directory, which some setups make (a per-project `bin` added by an
  environment hook); K1's design says the same of a driver binary, and
  the reference says so (UX-01). It can still choose the relay image by
  reference through `./warden.toml`, as on `main`; that is
  `project-config-trust`'s subject and unchanged here.
- **Which relays and executors the lookup does not find.** The
  container driver's relay is an image named by reference; Hyper-V's is
  compiled into the relay boot VHDX by `refresh-cache.sh`; vz's executor
  is the one baked at the last prepare (D2), found by the lookup then
  and not per build, so "an older executor is not delivered" is enforced
  at prepare. Each is in the record: the image with the ID it resolved
  to, the VHDX with its path and stamp, the baked executor with its
  version and digest from the image's own record.
- **What shows which file was used.** The passed-over line on stderr;
  `<output>/host-files.json`; `warden doctor --driver <name>`. A build's
  output says that a file other than the bundled one was used exactly
  when `found` is `PATH`. The digest recorded is of the bytes the build
  used: each file is read once into the work directory, and the check,
  the digest and every later use are of that copy, so a file replaced
  after the lookup is either the one recorded or the build's error.
  None of this is signed, and none of it prevents anything.
- **GLOBAL-08.** The orchestrator does not support a substituted relay
  or executor, and this design does not offer `PATH` as a way to
  substitute one. It is where an install puts BuildWarden's own files.
- **`sudo`.** `sudo warden image prepare --driver vz` and `sudo warden
  doctor` search the `PATH` that process has. Where sudo resets `PATH`
  (`secure_path`, checked on this Linux host), that is root's, and the
  copy beside `warden` is what is usually found; where sudo keeps the
  invoking user's `PATH`, the user's is searched as root. Either way
  prepare only copies the executor into the image and never runs it on
  the host. Which of the two macOS does by default was not checked here;
  the `macos-vz` step records it.

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
4. A `warden.toml` in the context with `relay_image = "dev"`, run by a
   `warden` with no development record: refused before anything is
   created, nothing built.
5. A file on `PATH` that states an older version, and one that states
   none: passed over, with the line, and the bundled copy runs.
6. A file of an artifact's name in a `PATH` entry after `warden`'s own
   directory: not used, whether the copy beside `warden` passes or not.
7. A file replaced after the lookup and before staging: the bytes staged
   and the digest recorded are of one file, or the build is refused.

## Validation plan

**Tier 0** (`make test`, every platform; TEST-01).

- The lookup, table-driven, over temporary directories and a `PATH`
  given to the function: no candidate; the first candidate passes; it
  fails and the copy beside `warden` passes; both fail (the error names
  both); a later `PATH` copy is ignored; a copy in an entry after
  `warden`'s own directory is not used, with the bundled copy passing
  and with it failing (one place in the error); a relative and an empty
  entry; a set split across two directories is not a candidate; a
  directory without the stamp is still a candidate, and its stamp fails
  only a build that boots the stamped file; Windows `PATH` splitting and
  the `.exe` names (build-tagged cases).
- The version reader over byte slices: the line present, absent,
  truncated, over-long, three times identical (one version), twice and
  differing (none); and the comparison table of **The version**,
  including pre-release precedence.
- Read once: the source file is replaced after the lookup; the staged
  bytes and the recorded digest are the checked ones, or the error of
  case 9.
- Each driver's staging with the test-only directory: the six tests
  named under **Development tooling** pass with placeholder files, no
  compiler and no use of the process's `PATH`; a missing file is the
  fixable error, before any work directory exists.
- The container driver's image choice as a table over (version, digest
  compiled in or not, `relay_image`, record present or not): which
  reference, and that no build is attempted; the errors of cases 1 and
  6.
- `warden clean`'s image filter: `warden-relay:<dead build id>` is
  removed and `warden-relay-dev:*`, `warden-holder-dev:*`,
  `warden-netns-dev:*` are not; `--cache` adds the two content-hash
  forms.
- `host-files.json` round-trips, with files, images, the prepared-image
  entry and the boot disk entry; the doctor block's lines.
- The transitional error (case 7) and its absence when the new name is
  found.
- The artifact list against `make artifacts`; `install.sh`'s install
  step against the old layout, the new one, and an archive with no
  `warden`.
- No `exec.Command` of `go` remains under `cmd/` or `driver/` (a test
  that reads the tree, as the stamp-script test does).

**Tier 1** (CI). The `build` job: every executable in `dist/` states
`dist/warden`'s version once; `goreleaser check`; the snapshot release,
each archive listed against the artifact list and each archived
executable's version line read (**Releases and install**). The
`integration` workflow (docker on Linux): `make build images`, then the
existing builds; `make images`, `warden clean`, and a build still runs; a
build with the executor copied to an absolute `PATH` directory records
`found: PATH`; with a recorded image removed, the error of case 1;
`project-config-trust`'s F9 case, and the same with `dist/warden` inside
the tree, with a recording `go` first on `PATH`. The `firecracker`
workflow (tier 2 in CI, on pull requests that reach its paths): its
builds pass with the assets placed by `make build`, and its doctor shows
the `Host files` block.

**Tier 2, native** (FLOW-09). Every driver's tree is reached, so each
host whose driver only it can run blocks. The cases that change `PATH`
run through `tools/native/host-files-cases.sh`.

| Host | Blocks | Cases |
|---|---|---|
| `macos-finch` | yes | `make build`, `make images CTR=finch`, run from the branch under test; doctor's block lists the three recorded images as present; a default build verifies, and `host-files.json` says `beside-warden` and gives three image IDs that are the recorded references'; no `go` is started (a recording `go` on `PATH`); `warden clean`, then a build still runs; with a recorded image removed, case 1's error before anything is created; a copy of the executor in a `PATH` directory before `warden`'s is used, one built with an older `VERSION` is passed over with the line, one after `warden`'s directory is not used; `relay_image = "dev"` and `WARDEN_IMAGE=<ref>` |
| `macos-vz` | yes | `sudo env` shows the `PATH` prepare runs with; `sudo dist/warden image prepare --driver vz` installs `dist/warden-io-darwin-arm64`, names it, and records it with the image; a build's `host-files.json` carries that record and the relay's path, version and digest, and the relay process is a copy of `dist/warden-relay-darwin-arm64`; with that file moved away, refused before any VM boots; no `go` is started |
| `windows-hyperv` | yes | `make build` under Git Bash makes `dist/warden-io-windows-amd64.exe`; a durable and an ephemeral build pass, and the seed's `warden-io.exe` has the digest in `host-files.json`, which also names the relay boot VHDX and its stamp; with the file moved away, case 2's error before any VM; a copy in a `PATH` directory is used and one in the working directory is not; `go test ./driver/...` on Windows; the relay boot VHDX is found as before |
| `qemu-any` | yes | the asset scripts, then `make build` places both sets with the stamp; a direct-boot build and a cloud-image build verify; the direct-boot build runs the appended executor (its digest in `host-files.json`), with an initramfs that still carries a baked one as well as a rebuilt one; assets moved away give case 3 before any VM; with the direct-boot stamp removed, a direct-boot build gets the "predates the configurable build link" error and a cloud-image build runs; run from a directory that is a buildwarden checkout with `tools/relay-vm/output/` filled, nothing of it is used |
| `container-any` | no, tracked | the Windows desktop: `go test`, `make images` under Git Bash, the relay container starts, a container build (`prebuilt-artifacts-windows`) |
| `macos-qemu` | no, tracked | a qemu Windows guest build with the Windows executor from the lookup (`prebuilt-artifacts-qemu-windows`) |

Firecracker has no native host that is not experimental (`linux-kvm`);
its CI workflow is its tier 2.

**What was run for this design, and what was not.** Run, on this Linux
host: the cross-builds quoted (`GOOS=darwin GOARCH=arm64 CGO_ENABLED=0
go build ./cmd/warden` fails; `./cmd/warden-io` and `./cmd/relay` build
for darwin/arm64, and `./cmd/warden-io` for windows/amd64 and
windows/arm64); `go version -m` on a Windows and a macOS binary, and `go
run`, `go build`, `go test` and `-buildvcs=false` in a scratch module,
for Decision 4's rejected alternative; the version line's four rows in a
scratch module (**The version**); goreleaser v2.20.2 in a scratch clone
(`check`, and the snapshot runs of the **Inventory**); the releases API,
the `v0.6.0` release run's log, and v0.5.0's linux/amd64 and
darwin/arm64 archives downloaded and listed. Run by the reviewers and
taken from their reports: the version line in this repository's own
three programs; `warden clean` removing `warden-relay:dev` with `main`'s
`dist/warden` and a docker daemon; the six tests found by making
`ModuleRoot` fail. Read and not run: every driver's build, the asset
scripts, `install.sh`, any container runtime, macOS and Windows. Not
verified: that `finch` and `podman` accept `--build-arg` and
`COPY --chmod` as docker does for `make images` (the holder's
Containerfile already relies on the latter under all three); a full
goreleaser snapshot with the macOS targets; what macOS's sudo does with
`PATH`.

## Implementation plan

Plan starts when the hold of a9 and a8 is released, and merges `main`
first. Under a9 (A) the item first returns to Design for **In K1's
structure**. Rule-file edits come first (LOOP-02).

1. `docs/development/bootstrap.md`: BOOT-02, the diagram label, the
   added paragraph and the qemu / Linux mapping cell, as approved
   (FLOW-14, BOOT-07).
2. The version package; `-trimpath` and the linker flag in the Makefile;
   `warden --version` reads it, and `warden-io` and `relay` gain
   `--version`, so each program reads the variable on a reachable path;
   a test that builds nothing reads the line from a byte slice, and the
   `build` job proves it on real `dist/` binaries for Linux, Windows and
   macOS targets (GLOBAL-04, GLOBAL-07).
3. The lookup, the check, read-once staging, the recorder and the
   test-only directory in `driver/`, with the tier-0 table (GLOBAL-03,
   GLOBAL-10, GLOBAL-11, UX-04).
4. Rework: every `GuestBinary` caller (rows 2a to 2g) asks the lookup,
   before anything is created (ORCH-01, UX-08). `relayvm.Find` and
   `StampPath` lose the tree; the stamp stays checked where it is today;
   the qemu and Firecracker fix messages are rewritten.
5. Rework: the container driver: the executor from the lookup, the
   development record, `relay_image = "dev"`, the image IDs; delete
   `goBuildLinux`, `buildRelayFromSource`, the embedded Containerfiles
   and the image removal; `clean`'s filter test and `--cache`
   (GLOBAL-09, UX-03).
6. Rework: qemu direct boot appends the executor;
   `tools/build-vm/build-initramfs.sh` stops compiling it (BOOT-02,
   BOOT-06).
7. Delete `ModuleRoot`, `GuestBinary`, the cache helpers, and
   `project-config-trust`'s setter and `TestMain`s; move the six tests
   to the test-only directory (GLOBAL-04).
8. `host-files.json`, the doctor block, the passed-over line, the vz
   prepare line and the prepared image's record (UX-02).
9. The transitional detection, in one file under
   `TRANSITIONAL(artifact-names)` (UX-03).
10. `make build` (the Windows executors, the asset copy), `make
    artifacts`, `make images` with `Dockerfile.relay`'s `--chmod`,
    `test-integration.sh`, the `build` job's version check, `Makefile`
    in the paths of `.github/workflows/integration.yml` and
    `.github/workflows/firecracker.yml`, and
    `tools/native/host-files-cases.sh` (TEST-02).
11. `.goreleaser.yaml` (the hook, its directory, the archive's files),
    `.gitignore`, `install.sh`, the snapshot step in the `build` job,
    and the two tier-0 tests of **Releases and install**.
12. Reference docs, `README.md`, `CONTRIBUTING.md`, `structure.md` and
    the overview (UX-01, FLOW-15).
13. `make test`, `make lint` and `GOOS=windows`, `GOOS=darwin make lint`
    (FLOW-17).

## Design review

No review yet.

Before the review, on 2026-11-15, two independent reviewers read the
design at `36e8051` (the code and the release; security, the rules and
the program). What their findings changed: the development images are
tied to the checkout that built them and no `warden clean` removes them
(Decision 6, a4); `install.sh` installs any published layout, the
release table states what v0.5.0 ships, and the release configuration's
faults are fixed where this item rewrites them and proven by a snapshot
in CI (Decisions 9 and 12, a7); every program reads its version line,
every build uses `-trimpath`, and identical repeats are one statement
(Decision 4); a stamp is a check and not a file of the set, applied only
where it is today (Decision 5); the record is `host-files.json`, covers
images, Hyper-V's boot disk and vz's baked executor, and the digest is
of the bytes used (Decision 10); `warden`'s own directory on `PATH` ends
the search (Decision 2); the order with K1 is a question (a9), with what
K1's structure changes listed; `alters_relationships` is argued on one
ground; GLOBAL-08, ORCH-03 and UX-07 are cited.

### 2026-11-15, first sitting, outcome: approved

- **r1, 2026-11-15: approved at `7a85a33`** (Jeff; page
  https://claude.ai/artifact/CRVXjYapJ26Tp9WnUhomxu). All nine questions
  were asked, each on its own, and answered as the design proceeds: a9
  (A), K1 first; a1 (A); a2 (A); a3 (A); a4 (A); a5 (A); a6 (A); a7 (A);
  a8 (A), the hold, which under a9 (A) names `driver-contract`'s merge.
  The Bootstrap comparison is accepted explicitly (BOOT-07), with
  BOOT-02. The Security note is accepted (GLOBAL-06). One correction to
  the text as he read it, made on his word in the sitting: the macOS
  arm64 release-build failure is not filed as this item's follow-up
  (**Handed on**; a7's text) but as an item of its own, `vz-nocgo-build`,
  started the same day, so that releases are not held behind this item.
  The hold is written by the session that records this entry: a blocking
  question `q1` with the step `f2-answer-q1` before `f2-plan`. When
  `driver-contract`'s PR has merged, the item returns to Design once for
  the delta K1's structure brings (a9 (A)), and that delta is reviewed.
- **rule change BOOT-02 approved at `7a85a33`**, as worded under **Rule
  changes**, in both settings (on `main`'s sentence, and on
  `driver-contract`'s approved sentence), with the Bootstrap comparison.

## Consolidation

Filled in at Publish.

## Rollback notes

None yet.

## Archive summary

Filled in at Publish.
