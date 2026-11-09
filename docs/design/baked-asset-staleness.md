---
feature: baked-asset-staleness
status: draft
drivers: [container, qemu, vz, hyperv, firecracker]  # qemu (relay and direct-boot assets checked before boot and in doctor), firecracker (kernel, relay and direct-boot initramfs), hyperv (relay boot VHDX: warden now checks the source, not only refresh-cache.sh), vz (the prepared image's stamp, its baked files moved to embedded files). container is declared conservatively (FLOW-02, TEST-02 caveat): driver/hostlink.go's LauncherStamp, in the shared driver/ root, is replaced, but the container driver has no baked boot asset and its behaviour is unchanged
seams: []                # the stamp is a file between this repository's own build tools and warden, not a GLOBAL-08 seam; the claim, the collector, executor-initialize, the ledger and the bootstrap model are unchanged
surfaces: [output]       # doctor's per-asset lines (a source stamp instead of a link-channel stamp) and the refusals' wording; no flag, subcommand or config key changes (UX-01, UX-03, UX-04)
security_boundary: none  # the stamp is a consistency check on assets the user built, not an integrity control; it moves no boundary (Security note kept short for the record, under Design)
alters_relationships: false  # every VM driver already refuses a stale launcher asset before boot and reports it in doctor (build-link-subnet Decision 6); this widens what is compared and which assets carry a stamp, inside each driver's existing role. No seam, no bootstrap-model change, no Driver/Session contract change
design_review: required  # FLOW-19: blocking native validation on four hosts and seven assumed questions
native_validation:
  - {host: qemu-any, blocking: true, reason: "reaches driver/qemu (the relay kernel and initramfs now checked before every build and BLOCKED in doctor; the direct-boot initramfs's stamp format) and the two scripts that build them (tools/relay-vm/build-initramfs.sh, tools/build-vm/build-initramfs.sh). No CI workflow boots qemu: the upgrade from main's unstamped assets, the rebuild, a cloud-image and a direct-boot build, the release-name layout beside the binary and the stale-manifest refusal run only on the macbook"}
  - {host: linux-kvm, blocking: true, reason: "reaches driver/firecracker (the kernel and the x86_64 relay initramfs now checked and BLOCKED in doctor, the direct-boot initramfs's stamp) and tools/firecracker-vm's two build scripts. firecracker.yml builds fresh assets on a hosted runner and covers the fresh path, but it is not yet required for merge and its header names the native linux-kvm session as the blocking validation; the upgrade from main's assets runs only natively"}
  - {host: windows-hyperv, blocking: true, reason: "reaches driver/hyperv (warden itself now compares the relay boot VHDX's source digest, where only refresh-cache.sh read a source stamp before) and tools/relay-vm/hyperv/refresh-cache.sh (content digest instead of git history, new stamp file, old stamps removed, .prev handling). Runs only on a Hyper-V host under Git Bash and WSL"}
  - {host: macos-vz, blocking: true, reason: "reaches driver/vz (the prepared image's stamp written by both prepare paths and compared before boot; warden-boot and the launchd plists moved from Go string constants to embedded files whose bytes must be unchanged). Runs only on an Apple-silicon Mac"}
  - {host: macos-qemu, blocking: false, reason: "a qemu Windows guest boots the same relay VM as a Linux guest, so the relay check is guest-agnostic and validated by qemu-any; qemu Windows builds already fail on main (backlog qemu-windows-guest-package), so a blocking step could not pass for reasons outside this feature"}
tier2_gaps:
  - {host: macos-qemu, gap: "a qemu Windows guest build refused on an unstamped relay initramfs and succeeding after the rebuild (backlog baked-asset-staleness-qemu-windows), once qemu-windows-guest-package is fixed"}
rules: [FLOW-01, FLOW-02, FLOW-05, FLOW-06, FLOW-07, FLOW-09, FLOW-12, FLOW-14, FLOW-15, FLOW-19, UX-01, UX-03, UX-04, BOOT-02, BOOT-06, BOOT-07, ORCH-01, GLOBAL-07, GLOBAL-08, TEST-01, TEST-02, TEST-03, TEST-04]
rule_changes: []
concepts: [stale-assets, launcher-stamp, source-digest, boot-assets, baked-assets, generated-manifest, relay-vm-assets, direct-boot, guest-kernel, relay-boot-vhdx, image-prepare, doctor, asset-cache, ci-cache-key, explicit-over-implicit]
related: [build-link-subnet.md, firecracker-driver.md, hyperv-windows-prepare.md, vm-driver-release-plan.md, unified-bootstrap.md]
---

# One staleness check for every baked boot asset

## Problem

Every VM driver boots files that a script or `warden image prepare` built
earlier from this repository: relay initramfs and kernels, direct-boot
launcher initramfs, the Firecracker guest kernel, the Hyper-V relay boot
VHDX and the vz prepared image. Each of them bakes something from the source
tree (an init, a launcher, a kernel config, the relay binary, `warden-io`,
`warden-boot`). When the source moves and the asset does not, the build
fails late and obscurely: a relay that never answers, an init that sets the
wrong address, a guest that never reports.

Today only part of this is checked, three different ways:

- **The link-channel stamp** (build-link-subnet Decision 6):
  `WARDEN_LINK_CHANNEL=1` beside the qemu and Firecracker direct-boot
  initramfs, the Hyper-V relay boot VHDX and the vz prepared image, compared
  with `buildlink.Channel` before boot and in doctor. It catches exactly one
  change (how a launcher reads the build link) and only when someone bumps
  the constant by hand. `hyperv-relay-per-build-channel` (Decision 9) is
  about to give Hyper-V its own constant, `relayBootChannel = 2`, for its
  second such change.
- **`relay-boot.source`**: `tools/relay-vm/hyperv/refresh-cache.sh` writes
  the last commit touching the relay's inputs beside the VHDX and rebuilds
  when it differs. Warden never reads it, so a Hyper-V build runs a stale
  relay unless the user (or `native-tick.sh`) ran the script first.
- **Nothing**: the qemu and Firecracker relay initramfs (which bake
  `tools/relay-vm/init`), the Firecracker kernel (which bakes
  `kernel-required.config`), the `warden-io` baked into the qemu direct-boot
  initramfs and the vz image, and the vz image's `warden-boot`.

build-link-subnet's Decision 10 deferred the general answer here, approved
by Jeff at its design review r3 (a15 (c) as the follow-up): one source digest
per baked asset, read by build and doctor, so a stale asset is refused before
anything boots, naming its rebuild. It absorbs the link-channel stamp and the
Hyper-V source stamp. It is due now because two items in flight change baked
inputs again (`hyperv-relay-per-build-channel` rewrites `tools/relay-vm/init`,
which every vm-mode relay bakes; `build-vm-busybox` changes how the qemu
direct-boot initramfs is built), and each would otherwise need its own
hand-bumped constant or none.

## Prior art

- [`build-link-subnet.md`](build-link-subnet.md) (archived), Decisions 6
  and 10, questions a3 and a15: the launcher stamp this design replaces, its
  per-driver table, the doctor split (BLOCKED for an asset every build uses,
  reported only for direct boot, added at Review r1), the vz `--image`
  outside the cache fix ("omit --image"), and the follow-up this item is.
  Reused: the check's place (before any VM boots, `driver.Fixable` naming the
  rebuild), the doctor split, the per-driver listing functions
  (`LauncherStamps`), the release-name layout (`<name>` plus a suffix beside
  the binary), the tier-0 test that every building tool writes its stamp.
  Departed from: a hand-bumped constant (`buildlink.Channel`), which only
  catches changes someone remembers to bump for.
- `tools/relay-vm/hyperv/refresh-cache.sh`: the only source stamp today, the
  input set it hashes (the relay's main-module packages from
  `go list -deps ./cmd/relay`, `go.mod`, `go.sum`, `tools/relay-vm` minus the
  script itself) and its interface (`--check`, `--force`, default refresh),
  which `tools/workflow/native/native-tick.sh` and `install-windows.ps1` call.
  Reused: the input set's shape and the interface. Departed from: a git
  commit as the digest (it needs git, and a dirty tree always rebuilds).
- [`hyperv-windows-prepare.md`](hyperv-windows-prepare.md), Manifest: a
  prepared Windows layer records `inputs` (sha256 of the unattend,
  `warden-prepare.ps1`, the tuning list) and `warden_version`. Precedent for
  recording what a baked asset was made from; those layers carry only the
  generic `warden-run` hook (BOOT-06) and stay out of scope (a2).
- [`firecracker-driver.md`](firecracker-driver.md), Decision 10: the pinned
  kernel and the two x86_64 initramfs, "source-tree built, as qemu's relay
  assets are today", release packaging of VM assets recorded as a gap. This
  design keeps assets source-tree built and leaves a packaging path open
  (the stamp is written by the same tool whoever runs it).
- `.github/workflows/release.yml` and `.goreleaser.yaml`: no VM boot asset
  ships in a release (only `warden`, `warden-io`, `warden-relay`), so every
  asset is built from a source tree, and a release `warden` can only meet
  assets someone built from some tree. The container driver's support images
  are already compiled into `warden` by digest (`-X ...holderImageRef`), the
  precedent for "the binary knows what it expects".
- [`unified-bootstrap.md`](unified-bootstrap.md) and `bootstrap.md` D2: the
  vz image bakes `warden-io` and its launchd daemon, a tracked deviation from
  BOOT-02 and BOOT-06 ("a stale one lacks newer baseline steps"). This design
  does not change the deviation; it makes its staleness refused instead of
  silent (Decision 9). It also finds a second, unregistered instance: the
  qemu direct-boot initramfs bakes `warden-io` (a6).
- [`vm-driver-release-plan.md`](vm-driver-release-plan.md), "Image Lifecycle
  Management": images go stale and need an update path; integrity
  verification by content hash. Not taken up here: this is about source
  consistency, not tamper evidence (Security note).
- In flight, read on their branches, not yet on `main`:
  `vz-image-prepare-refresh` (phase 6: the active image, refresh in place,
  `imageStamp(dir, *ImageCache)`), `build-vm-busybox` (phase 3: the direct-boot
  script fetches its own busybox, keeps the relay cache's kernel and
  modules), `hyperv-relay-per-build-channel` (phase 2: Decision 9's
  `relayBootChannel = 2`, the seed-reading relay init). Coordination is in
  Decision 10.
- Jeff's design principles (queue `design-principles`, 2026-11-08): least
  surprise, explicit over implicit (no silent fallbacks), minimal magic
  (predictable over resilient). They decide a1, a4 and Decision 7 here.

## Decisions

1. **Warden knows each asset's expected digest from a manifest compiled into
   it; it never computes one at run time** (question a1). The package
   `driver/bootasset` holds a generated file, `manifest.go`, mapping each
   asset ID to the sha256 source digest of its inputs at the commit warden
   was built from. A tier-0 test recomputes every digest from the tree and
   fails, naming `make generate`, when the committed manifest differs, so a
   committed tree's manifest is always its own. A release `warden` therefore
   knows exactly what its tag's assets are made from, with no source tree on
   the machine; a `warden` built from a tree expects that tree's assets.

   *Rejected:* computing the expected digest at run time from a source tree
   warden finds (`driver.ModuleRoot` walks up from the executable and then
   the working directory, so the verdict would depend on where you stand,
   and a release has no tree: magic either way). Hand-bumped per-asset
   constants, the link-channel stamp generalized (they catch only the changes
   someone bumps for, the failure this item exists to remove). A digest of
   the built file (Go builds and Alpine downloads are not reproducible across
   machines, so it cannot be known in advance).

2. **The assets, their IDs and their rebuilds** (question a2). One ID per
   thing a tool builds; every file warden boots maps to one ID.

   | ID | Files warden boots (source tree / beside the binary) | Driver | Built by (the fix a refusal names) | Every build uses it |
   |---|---|---|---|---|
   | `relay-vm-aarch64` | `tools/relay-vm/output/{vmlinuz,initramfs.cpio.gz}` / `relay-vm-vmlinuz`, `relay-vm-initramfs.cpio.gz`, and `build-vm-vmlinuz` (the direct-boot kernel is the relay's) | qemu | `tools/relay-vm/build-initramfs.sh` | yes |
   | `relay-vm-x86_64` | `tools/relay-vm/output/x86_64/initramfs.cpio.gz` / `firecracker-relay-initramfs.cpio.gz` | firecracker | `ARCH=x86_64 NO_MODULES=1 tools/relay-vm/build-initramfs.sh` | yes |
   | `firecracker-kernel` | `tools/firecracker-vm/output/vmlinux` / `firecracker-vmlinux` | firecracker | `tools/firecracker-vm/build-kernel.sh` | yes |
   | `qemu-direct-boot` | `tools/build-vm/output/initramfs.cpio.gz` / `build-vm-initramfs.cpio.gz` | qemu | `tools/build-vm/build-initramfs.sh` | no: builds with no disk image |
   | `firecracker-direct-boot` | `tools/firecracker-vm/output/initramfs.cpio.gz` / `firecracker-build-initramfs.cpio.gz` | firecracker | `tools/firecracker-vm/build-initramfs.sh` | no: builds with no disk image |
   | `hyperv-relay-boot` | `<cache>/relay-boot.vhdx`, or `$WARDEN_RELAY_BOOT_VHDX` | hyperv | `tools/relay-vm/hyperv/refresh-cache.sh` | yes |
   | `vz-image` | the active prepared image's `disk.img`, or the `--image` disk | vz | `sudo warden image prepare --driver vz` (refresh in place); for an `--image` disk outside the image cache, "omit --image" as today | yes |

   Out of scope, each for a stated reason: the prepared Windows layers on qemu
   and Hyper-V (they carry only the generic `warden-run` hook, BOOT-06; the
   per-build `warden-run.ps1` and `warden-io.exe` come on the seed, and their
   freshness is the existing grace and refresh policy); qemu's and
   Firecracker's Linux cloud images (nothing of warden's is baked, the seed
   carries it); the container driver's holder and sidecar images (already
   compiled in by digest); the vz `Image` kernel the relay script also writes
   (used only by a vz integration test, not by a driver).

3. **The digest is over the content of declared inputs, never git history**
   (question a3). Each asset declares, in `driver/bootasset`'s input table:
   explicit repository files (paths or globs that must each match), and for
   each Go binary it bakes, the package and target (`GOOS`, `GOARCH`,
   `CGO_ENABLED=0`). The digest is sha256 over a canonical listing, sorted,
   one line per item:

   ```
   bootasset/v1                       format version: a change re-digests everything
   asset <id>
   file <repo-relative/slash/path> <sha256 of the content, CRLF -> LF>
   go <go directive of go.mod>
   gofile <path> <sha256, CRLF -> LF>  every Go and embedded file of each main-module
                                       package in the binary's deps for its target
   module <path>@<version>             each external module in those deps (with its
                                       replacement, if any)
   ```

   Content, not commits: a dirty tree digests like the commit it would
   become, git is not needed, and a comment-only edit is a change (predictable
   over clever). CRLF is normalized so a Windows checkout (Git Bash,
   `refresh-cache.sh`) digests like Linux (GLOBAL-07). Only the modules the
   binary links count, so a dependency bump elsewhere rebuilds nothing.
   External downloads a script fetches unpinned (Alpine 3.21's current
   `linux-virt`, `iptables`; the minirootfs by name) are inputs only through
   the script that names them: drift on the mirror is not detected, as today.

   The initial input sets (Plan refines them; the coverage test in the
   Validation plan keeps them honest):

   | ID | Files | Go binaries |
   |---|---|---|
   | `relay-vm-aarch64`, `relay-vm-x86_64` | `tools/relay-vm/build-initramfs.sh`, `tools/relay-vm/init` | none (the relay is appended per build) |
   | `firecracker-kernel` | `tools/firecracker-vm/build-kernel.sh`, `tools/firecracker-vm/kernel-required.config` (`kernel.config` is the build's output, not an input) | none |
   | `qemu-direct-boot` | `tools/build-vm/build-initramfs.sh`, `tools/build-vm/init`, `tools/relay-vm/build-initramfs.sh` (its cache supplies the kernel modules) | `cmd/warden-io` linux/arm64 |
   | `firecracker-direct-boot` | `tools/firecracker-vm/build-initramfs.sh`, `tools/firecracker-vm/init` | none (`warden-io` is appended per build) |
   | `hyperv-relay-boot` | `tools/relay-vm/init`, `tools/relay-vm/hyperv/build.sh`, `build-boot-vhdx.sh`, `fetch-cache.ps1` | `cmd/relay` linux/amd64 |
   | `vz-image` | `driver/vz/guest/*` (Decision 9) | `cmd/warden-io` darwin/arm64 |

   *Rejected:* the last commit touching the inputs (today's
   `refresh-cache.sh`: needs git and a full history, and `-dirty` forces a
   rebuild every run). Whole directories plus `go.mod`/`go.sum` (any
   dependency bump anywhere rebuilds every asset; scratch files in `.cache/`
   and `output/` would have to be excluded by convention).

4. **A stamp beside every boot file: `<file>.source`** (questions a2, a5).

   ```
   WARDEN_ASSET=relay-vm-aarch64
   WARDEN_SOURCE=sha256:<64 hex>
   ```

   One rule everywhere: in the source tree (`output/initramfs.cpio.gz.source`),
   beside the binary under the release name
   (`relay-vm-initramfs.cpio.gz.source`), in the Hyper-V cache
   (`relay-boot.vhdx.source`, or `<override>.source`), and in the vz image
   (`disk.img.source`, or `<--image disk>.source`). A per-file stamp, not one
   per output directory, because `tools/firecracker-vm/output/` holds the
   kernel and the direct-boot initramfs from two different scripts, and
   because files copied beside the binary travel one by one. A tool that
   writes several boot files (the relay script: `vmlinuz` and
   `initramfs.cpio.gz`) stamps each.

   The stamp names its asset, so a file stamped for another asset (an
   x86_64 relay initramfs copied over the aarch64 name) is refused as such,
   not as merely stale.

5. **One tool writes and checks stamps: `go run ./tools/bootasset`**
   (question a1). Subcommands:

   - `stamp <id> <file>...`: computes `<id>`'s digest from the tree the tool
     runs in, **refuses when that tree's committed `manifest.go` disagrees**
     ("the manifest is stale for `<id>`: run `make generate`, then rebuild
     warden"), and otherwise writes `<file>.source` for each file. The
     refusal is what keeps a developer from building an asset that their
     own `warden` would then refuse: the two can only disagree when the
     manifest is stale, and the fix is the same either way.
   - `check <id> <file>`: exit 0 when `<file>.source` names `<id>` with the
     tree's digest, 1 otherwise with the reason (for `refresh-cache.sh`).
   - `generate`: writes `driver/bootasset/manifest.go` (`//go:generate` in
     the package; `make generate` runs it).
   - `digest <id>`: prints the digest (for a CI cache key, debugging).

   Each build script calls `stamp` once, at the end, after its outputs are in
   place, so an interrupted build leaves no stamp (and is refused). This adds
   `go` to the dependencies of `tools/relay-vm/build-initramfs.sh` and the two
   `tools/firecracker-vm` scripts; `tools/build-vm/build-initramfs.sh` and
   `refresh-cache.sh` already run `go`. The runtime check lives in
   `driver/bootasset` too, but warden's code path reads files only: it
   imports no `go list` and needs no tree.

   *Rejected:* computing the digest in shell (`sha256sum` over a file list):
   it cannot follow a Go binary's package set, and two implementations of
   one digest would drift. A hidden `warden` subcommand (the scripts would
   need a built warden first, and the warden they find may not be the
   tree's).

6. **Build refuses, doctor reports, with build-link-subnet's split; nothing
   bypasses it** (question a4). Before any VM boots, a driver checks the
   stamp of every boot file that build will boot: the relay assets on every
   qemu and Firecracker build, the Firecracker kernel, the direct-boot
   initramfs only on a direct-boot build, the Hyper-V relay boot VHDX, the vz
   image. Missing, malformed, another asset's, or a different digest is a
   `driver.Fixable` naming the asset's rebuild (table, Decision 2), with the
   two digests shortened to 12 hex digits:

   ```
   ERROR: qemu relay VM initramfs was built from other sources than this warden
          (relay-vm-aarch64: built from sha256:3f9a0c1e7b2d, this warden expects
          sha256:a41c99e0d3f5)
   To fix: tools/relay-vm/build-initramfs.sh
   ```

   Found beside the binary, the fix adds "then copy `<name>` and
   `<name>.source` beside warden". Missing reads "has no source stamp (built
   before warden checked boot assets, or copied without its `.source`)".
   The message does not guess which side is newer: rebuilding the asset from
   the tree `warden` was built from is the fix in both directions.

   `warden doctor --driver <d>` prints one line per asset:
   `<what>: source current (<id> sha256:<12>)`, or `missing`/`stale` with
   both digests and the rebuild; `BLOCKED:` and a non-zero exit for an asset
   every build uses (the table's last column), reported only for the
   direct-boot initramfs. An asset not found is reported as today
   (`not found`).

   No flag, config key or environment variable skips the check. A
   developer iterating on an input rebuilds the asset with its script (which
   refuses until `make generate` has run) and rebuilds warden: two commands
   they already run, and the outcome never depends on a setting they forgot.

   *Rejected:* a warning instead of a refusal for the relay initramfs ("an
   old init often still works": exactly the resilience-over-predictability
   trade the principles reject). An escape hatch
   (`WARDEN_ALLOW_STALE_ASSETS`): a silent fallback by another name.

7. **The link-channel stamp is retired, not kept beside the new one**
   (question a5). `buildlink.Channel`, `StampKey`, `ErrNoStamp`,
   `ReadStamp`, `CheckStamp`, `StampState`, `driver.CheckLauncherStamp`,
   `relayvm.StampPath`'s `link-channel` names, `vz`'s `linkStampName` and
   `writeLinkStamp`, every script's `WARDEN_LINK_CHANNEL=1` line and the
   `TestStampScriptsWriteChannel` test go; `driver.LauncherStamp` becomes
   `driver.BootAsset` (same fields plus `ID`; `Check`, `Blocks`, `Report`
   keep their roles), and each driver's `LauncherStamps()` becomes
   `BootAssets()`. Any change to a launcher is now an input change, so the
   reason the channel existed is covered without a constant.

   Version skew, every direction refused before any VM boots:

   | Warden | Asset | Outcome |
   |---|---|---|
   | new | built before this change (link-channel only, or nothing) | missing `.source`: refused, rebuild once |
   | new | built from another commit's inputs | stale: refused, rebuild from warden's tree |
   | old (link-channel) | built by a new tool (`.source` only) | the old warden finds no `link-channel` and refuses with its own rebuild, which run from its tree builds the matching asset |
   | either | matching | builds |

   *Rejected:* writing both stamps for a while. An older warden would accept
   a new asset by its link-channel line and boot something built from other
   sources, the failure this design removes.

8. **Hyper-V: warden reads the source stamp, and `refresh-cache.sh` keeps its
   interface** (questions a3, a5). `refresh-cache.sh` decides "current" with
   `go run ./tools/bootasset check hyperv-relay-boot "$disk"` instead of
   `git log -1`, writes `relay-boot.vhdx.source` with `stamp` after
   installing the disk, moves the previous disk and its stamp to `.prev`
   together, and deletes the retired `relay-boot.source` and
   `relay-boot.link-channel` (and their `.prev`). Its modes (`--check` exit 1
   when stale, `--force`, default refresh) are unchanged, so
   `native-tick.sh` and `install-windows.ps1` need no change. A tree with
   uncommitted but unchanged inputs no longer rebuilds every run. The driver
   checks `relay-boot.vhdx.source` (or `$WARDEN_RELAY_BOOT_VHDX.source`) in
   `StartBuild` and doctor, `Required` as today.

9. **vz: prepare stamps the warden's own digest, and the files it bakes
   become embedded files** (questions a1, a2). Both prepare paths (restore,
   and refresh in place from `vz-image-prepare-refresh`) write
   `disk.img.source` with `bootasset.Expected("vz-image")`: the warden doing
   the prepare is the builder, so its manifest is the truth for what it
   wrote. To make that set exact, the content prepare writes into the disk
   (`warden-boot`, its LaunchDaemon plist, the agent plist, the
   setup-bypass markers), today Go string constants in `driver/vz/image.go`,
   moves byte for byte into `driver/vz/guest/` and is read through
   `go:embed`; the `vz-image` inputs are that directory plus `cmd/warden-io`
   darwin/arm64. An edit to unrelated vz code then re-prepares nothing. The
   `warden-io` prepare installs comes from `driver.GuestBinary` as today
   (beside the binary, else the cache, rebuilt when older than `go.sum` or
   the warden binary); that cache's own staleness rule is unchanged here and
   named under Design, "Limits".

   D2 itself is unchanged: `warden-io` is still baked. What changes is that a
   stale one is refused before boot with the in-place refresh as the fix,
   instead of running a build that lacks newer baseline steps. No edit to
   `bootstrap.md` is needed (its D2 text stays accurate), so no rule change.

10. **Coordination with the items in flight** (question a5). All three land
    before this one is implemented (they are ahead in FLOW-05 order), and
    Plan works from `main` as it is then.

    - `hyperv-relay-per-build-channel` (Decision 9 there): its
      `relayBootChannel = 2` and the `relay-boot.link-channel` value it bumps
      are retired here; its init change is simply a new `hyperv-relay-boot`
      digest (and new `relay-vm-*` digests), so no constant is bumped by
      anyone again. Its version-skew rows (an old warden refusing a new disk)
      still hold through Decision 7's table. If this design were to land
      first instead, that item's Decision 9 becomes unnecessary, and its
      design return would drop it.
    - `vz-image-prepare-refresh`: `imageStamp(dir, *ImageCache)`, the active
      image and `launcherStamps(cache)` are the shapes this design renames
      (`bootAssets(cache)`); its refresh-in-place path writes the new stamp.
    - `build-vm-busybox`: the direct-boot script's new busybox download is
      part of `tools/build-vm/build-initramfs.sh`, already an input; its
      `linux-virt.apk` from the relay cache is why the relay script is one
      too.

11. **No ledger change.** The ledger already records each boot layer's
    sha256 in the `vm-disk-chain/v1` descriptor (`driver/envdesc.go`), which
    is what a verifier needs; the source digest says nothing more about what
    the build consumed. Adding it would change the ledger format (a
    GLOBAL-08 seam) for no verification gain.

12. **Follow-ups found here, named for intake, not designed** (questions a6,
    a7).

    - `qemu-direct-boot-per-build-executor`: the qemu direct-boot initramfs
      bakes `warden-io` (`tools/build-vm/build-initramfs.sh` cross-compiles
      it into the rootfs), a second instance of D2 that `bootstrap.md`'s
      register does not list. Here its digest includes `warden-io`'s sources,
      so a `warden-io` change means one direct-boot rebuild. Appending
      `warden-io` per build, as Firecracker does
      (`relayvm.AppendFileCPIO`), would bring it into line with BOOT-02 and
      drop that input; registering it as D4 instead would be a model change
      (BOOT-07).
    - `native-asset-refresh` (a process item: `tools/workflow/` is machinery,
      FLOW-14): `native-tick.sh` refreshes only the Hyper-V VHDX before a
      session. With the relay assets now checked, a native session on the
      macbook or the Linux host is refused after any change to
      `tools/relay-vm/init` until someone rebuilds. The tick could run
      `tools/bootasset check` for each asset its hosts use and rebuild the
      stale ones, as it does for Hyper-V.

## Questions

### Blocking

None.

### Assumed

Each is confirmed or turned into a change at the design review (FLOW-19).

- **a1. Where warden gets the expected digest** (Decisions 1, 5, 9).
  Proceeding with a generated manifest compiled into warden, kept current by a
  tier-0 test, with the stamp tool refusing to stamp from a tree whose
  manifest is stale; vz prepare stamps its own manifest's value. Options:
  (a) as stated; (b) warden computes the digest at run time from a source
  tree it finds (depends on the working directory, impossible for a
  release); (c) hand-bumped per-asset constants (the link-channel stamp,
  generalized). Recommended: (a).
- **a2. The asset set and the stamp's place** (Decisions 2, 4). Proceeding
  with the seven assets of Decision 2 (including the Firecracker kernel,
  which the queue's source does not name but which bakes an in-tree config),
  a `<file>.source` beside every boot file, and the prepared Windows layers,
  the Linux cloud images and the container support images out of scope.
  Options: (a) as stated; (b) without the Firecracker kernel (its rebuild is
  a kernel compile); (c) also the prepared Windows layers (their `warden-run`
  registration). Recommended: (a).
- **a3. What the digest covers** (Decisions 3, 8). Proceeding with the
  CRLF-normalized content of declared files, the main-module Go and embedded
  files of each baked binary's packages for its target, its external
  modules' versions and the `go` directive; never git history; a coverage
  test classifying every tracked file under the asset tools' directories.
  Options: (a) as stated; (b) the last commit touching the inputs (today's
  `refresh-cache.sh`); (c) whole directories plus `go.mod`/`go.sum`.
  Recommended: (a).
- **a4. Refuse, report, no bypass** (Decision 6). Proceeding with a refusal
  before any VM boots for every boot file the build uses, doctor BLOCKED for
  the assets every build uses and reported only for the direct-boot
  initramfs, and no flag, key or variable that skips the check. The upgrade
  cost: every qemu and Firecracker user rebuilds the relay assets once (new:
  they were never stamped), Firecracker users the kernel once, and everyone
  else their stamped assets once, as for build-link-subnet. Options: (a) as
  stated; (b) warn only for the relay initramfs; (c) an escape-hatch
  environment variable. Recommended: (a).
- **a5. Retire the link-channel stamp outright** (Decisions 7, 8, 10).
  Proceeding with removing `buildlink.Channel` and every `link-channel` file,
  Hyper-V's `relay-boot.source` and `relay-boot.link-channel` replaced by
  `relay-boot.vhdx.source`, `relayBootChannel` (from
  `hyperv-relay-per-build-channel`) absorbed, and no period writing both
  stamps. Options: (a) as stated; (b) write both stamps for one release.
  Recommended: (a).
- **a6. The qemu direct-boot `warden-io`** (Decision 12). Proceeding with
  `cmd/warden-io` as an input of `qemu-direct-boot`, and
  `qemu-direct-boot-per-build-executor` named as a follow-up for intake.
  Options: (a) as stated; (b) append `warden-io` per build here; (c) register
  it as D4 here (a BOOT-07 model change with a rule change). Recommended: (a).
- **a7. Native machines' assets** (Decision 12). Proceeding with
  `native-asset-refresh` named as a process follow-up; until it lands a
  refused native session rebuilds by hand, as the refusal says. Options:
  (a) as stated; (b) no follow-up. Recommended: (a).

## Design

### `driver/bootasset` (runtime: reads only)

```
driver/bootasset/
  bootasset.go
    type ID string                      relay-vm-aarch64, relay-vm-x86_64, firecracker-kernel,
                                        qemu-direct-boot, firecracker-direct-boot,
                                        hyperv-relay-boot, vz-image
    IDs() []ID
    Expected(id) string                 "sha256:<hex>" from manifest.go
    StampPath(file) string              file + ".source"
    ReadStamp(path) (Stamp{ID, Source}, error)   missing -> ErrNoStamp; malformed -> error
    Check(file, id) error               nil | ErrNoStamp | wrong asset | different source
    State(file, id) string              "current" | "missing" | "stale"
    WriteStamp(file, id, source) error  temp file + rename
  inputs.go                             the input table (Decision 3), data only
  manifest.go                           // Code generated by tools/bootasset; DO NOT EDIT.
  digest/                               computes digests (go list, file hashing);
                                        imported by tools/bootasset and tests, never by warden
```

### The tool (`tools/bootasset`)

```
go run ./tools/bootasset stamp <id> <file>...
    root := module root of the tool's own source
    d := digest.Compute(root, id)
    if d != bootasset.Expected(id): fail "manifest stale for <id>: make generate, then rebuild warden"
    for each file: WriteStamp(file, id, d)
go run ./tools/bootasset check <id> <file>      exit 0 if Check against digest.Compute(root, id)
go run ./tools/bootasset generate               rewrite driver/bootasset/manifest.go
go run ./tools/bootasset digest <id>            print digest.Compute(root, id)
```

### The scripts

```
tools/relay-vm/build-initramfs.sh   end: stamp relay-vm-$ARCH output/[x86_64/]initramfs.cpio.gz
                                         (+ output/vmlinuz for aarch64)
tools/build-vm/build-initramfs.sh   end: stamp qemu-direct-boot output/initramfs.cpio.gz
tools/firecracker-vm/build-initramfs.sh   end: stamp firecracker-direct-boot output/initramfs.cpio.gz
tools/firecracker-vm/build-kernel.sh      end: stamp firecracker-kernel output/vmlinux
tools/relay-vm/hyperv/refresh-cache.sh
    current := [ -f disk ] && bootasset check hyperv-relay-boot disk     (no git log)
    --check: exit 0 / 1 with the reason; --force: rebuild regardless
    rebuild; disk, disk.source -> .prev; install; bootasset stamp hyperv-relay-boot disk
    rm -f relay-boot.source relay-boot.link-channel and their .prev
```

### Per driver (`overview.md`'s "before anything boots" block)

```
before anything boots:
    refuse a build link that holds a host interface address
    check the source stamp of every boot file this build boots
        (driver.BootAsset.Check: <file>.source names the asset and holds
         bootasset.Expected(id)); missing, another asset's or different
        -> Fixable(the asset's rebuild; beside the binary: and copy it with its .source)
    qemu:        relay-vm-aarch64 (kernel, initramfs); direct boot: + qemu-direct-boot
    firecracker: firecracker-kernel, relay-vm-x86_64; direct boot: + firecracker-direct-boot
    hyperv:      hyperv-relay-boot (relay-boot.vhdx, or $WARDEN_RELAY_BOOT_VHDX)
    vz:          vz-image (the active image's disk.img, or --image; outside the
                 image cache: omit --image)

vz image prepare (restore, refresh in place): write disk.img.source = Expected(vz-image)

doctor (each VM driver), per BootAsset:
    "<what>: source current (<id> sha256:<12>)"
    "[BLOCKED: ]<what>: source missing|stale (...); rebuild it: <fix>"   exit non-zero
        when BLOCKED (every-build assets); direct-boot initramfs reported only
```

`relayvm.Asset` gains `ID`; `relayvm.Find` is unchanged (all from beside the
binary or all from the tree), and the check takes each found path's
`<path>.source`, so the stamp always comes from where the file came from.

### Every site that changes

| Site | Change | Decision |
|---|---|---|
| `driver/bootasset/` (new) | IDs, input table, generated manifest, stamp read/check/write; `digest/` subpackage | 1, 3, 4 |
| `tools/bootasset/` (new) | `stamp`, `check`, `generate`, `digest` | 5 |
| `Makefile` | `generate` target | 1 |
| `driver/buildlink/buildlink.go` | stamp API and `Channel` removed; package comment | 7 |
| `driver/hostlink.go` | `LauncherStamp`, `CheckLauncherStamp` -> `BootAsset` | 6, 7 |
| `driver/relayvm/assets.go` | `Asset.ID`; `StampPath` removed | 4 |
| `driver/qemu/driver.go` | relay assets checked in `resolveRelayAssets`; `BootAssets()` | 2, 6 |
| `driver/firecracker/preflight.go` | kernel and relay checked; `BootAssets()` | 2, 6 |
| `driver/hyperv/buildassets.go` | `relay-boot.vhdx.source`; `relayBootChannel` (if landed) removed | 8, 10 |
| `driver/vz/driver.go`, `image.go`, `prepare.go`, `guest/` (new) | stamp, embedded guest files | 9 |
| `cmd/warden/host.go` | doctor iterates `BootAssets()` | 6 |
| five build scripts | `stamp` at the end, `WARDEN_LINK_CHANNEL` lines removed | 5, 7 |
| `.github/workflows/firecracker.yml` | cache key from `driver/bootasset/manifest.go`; paths `driver/bootasset/**`, `tools/bootasset/**` | 3 |
| `.github/workflows/auto-tidy.yml` | `make generate` after `make tidy` | 1 |
| docs: `overview.md`, `cli.md`, `drivers.md`, `installation.md`, `first-build.md`, `CONTRIBUTING.md`, `structure.md` | stamp wording, `make generate`, the asset table | UX-01, FLOW-15 |

None of these is a rule-change path (FLOW-14): nothing under
`docs/development/`, `tools/tier1/`, `tools/workflow/`, `.claude/` or
`docs/design/TEMPLATE.md`, and neither workflow edited runs tier 1,
`golangci-lint` or `tools/workflow`.

### Limits

- External inputs a script fetches by "current version" (Alpine's
  `linux-virt`, `iptables`) drift without a digest change, as today; pinning
  them is a separate concern.
- The qemu direct-boot initramfs's kernel modules must match the relay's
  `vmlinuz`. Both come from the relay script's cache; a rebuild of one
  without the other after Alpine moves is not detected.
- vz: the stamp asserts what the preparing warden expected; the `warden-io`
  it installed is `GuestBinary`'s, whose cache is mtime-checked.
- The digest is not an integrity check: a stamp can be copied or written by
  hand (Security note).

### Security note (kept for the record; `security_boundary: none`)

The stamp moves no boundary. It is a consistency check on assets the user
built: anyone who can write the asset can write its stamp, and nothing
treats a current stamp as evidence of integrity. What the build actually
booted stays in the ledger (each layer's sha256). The change helps posture
indirectly: a relay initramfs whose init predates a fix in
`tools/relay-vm/init` (iptables rules, the link block) is now refused
instead of run. No TEST-08 case is added.

## Cross-driver impact

- **qemu.** Every build now checks the relay kernel and initramfs
  (`relay-vm-aarch64`), BLOCKED in doctor; direct boot also checks
  `qemu-direct-boot`. Users rebuild the relay assets once (and the
  direct-boot initramfs once if they use it), then again only when an input
  changes (`tools/relay-vm/init` or the script; for direct boot also
  `warden-io`'s sources).
- **firecracker.** The kernel and the x86_64 relay initramfs are checked on
  every build and BLOCKED in doctor; the direct-boot initramfs as before but
  by digest. One rebuild of each (the kernel's is a compile, a few minutes
  with its cache).
- **hyperv.** Warden itself refuses a stale relay boot VHDX, which before only
  `refresh-cache.sh` noticed. One refresh, then one per change to the relay's
  inputs, as `refresh-cache.sh` already did.
- **vz.** The image's stamp is a digest; one refresh in place, then one per
  change to `warden-io` or the guest files (D2's cost, now visible).
- **container.** Unaffected: no baked boot asset; declared for the
  `driver/` root change only (ORCH-01).

## Validation plan

**Tier 0** (TEST-01, deterministic, TEST-04):

1. `TestManifestCurrent`: every ID's digest computed from the tree equals
   `manifest.go`; the failure names `make generate`.
2. `TestInputsCover`: every git-tracked file under `tools/relay-vm/`,
   `tools/build-vm/`, `tools/firecracker-vm/` and `driver/vz/guest/` is an
   input of some asset or on the table's named not-input list with a reason
   (e.g. `refresh-cache.sh`, the `.ps1` diagnostics, `kernel.config`,
   `kernel_config_test.go`), and every declared glob matches a file; a new
   file cannot escape silently.
3. `TestScriptsStamp`: each of the five scripts calls `tools/bootasset stamp`
   with its own ID exactly once, after its last output is written, and none
   names `WARDEN_LINK_CHANNEL`; `refresh-cache.sh` decides "current" with
   `check` and runs no `git log` (replaces `TestStampScriptsWriteChannel`).
4. Digest unit tests: CRLF and LF content digest alike; a comment edit
   changes the digest; reordering inputs does not; an unrelated module bump
   does not change `hyperv-relay-boot`; `bootasset/v1` and the ID are in the
   listing.
5. Stamp tests: missing, malformed, another asset's, different, current;
   `State` words; `WriteStamp` is atomic.
6. The `stamp` subcommand refuses when the manifest disagrees (digest
   injected through a seam, no temp module copy).
7. Per driver: each driver's pre-boot order test asserts the asset check
   precedes the first VM start and covers the files the build boots (relay
   always, direct boot only without an image); `BootAssets()` lists the
   right IDs and `Required` flags; the vz prepare paths write
   `disk.img.source`; the embedded `driver/vz/guest/` files equal the former
   constants byte for byte (a golden captured before the move).
8. A stray scan: no `link-channel`, `WARDEN_LINK_CHANNEL` or
   `relay-boot.source` left outside the archived designs and the cleanup
   lines of `refresh-cache.sh`.

**Tier 1**: the integration workflow runs (the `driver/` root changes);
the container driver is unchanged.

**Tier 2, CI**: `firecracker.yml` builds the assets with the new scripts
(cache keyed on `manifest.go`) and runs its builds, so the fresh path is
covered on every PR reaching it.

**Tier 2, native** (FLOW-09; blocking unless marked). Each on the branch
after consolidation, evidence under `native/<host>/<date>/`:

- **qemu-any** (blocking): (1) with main's assets in the tree: doctor
  `--driver qemu` exits non-zero with the relay kernel and initramfs
  BLOCKED (missing) naming `tools/relay-vm/build-initramfs.sh` and the
  direct-boot initramfs reported (missing) only; a cloud-image Linux build is
  refused before any VM with the same fix. (2) Run the relay script: it
  writes `vmlinuz.source` and `initramfs.cpio.gz.source`; doctor current;
  the cloud-image build succeeds and its ledger verifies. (3) A direct-boot
  build (no image) is refused naming `tools/build-vm/build-initramfs.sh`;
  after it, the build succeeds. (4) Edit a comment in `tools/relay-vm/init`:
  the relay script fails at its stamp step naming `make generate`; after
  `make generate` and `make build`, doctor reports the relay assets stale
  (two digests) and a build is refused; the relay script then stamps and the
  build succeeds; revert all three. (5) Release layout: `warden` and the
  release-named assets copied to a directory outside any tree without their
  `.source` files are refused naming the rebuild and the copy; with them,
  the build succeeds.
- **linux-kvm** (blocking): (1) with main's assets: doctor BLOCKED on the
  kernel and the relay initramfs, direct-boot reported; a build is refused
  before any VMM starts. (2) Build the kernel, the x86_64 relay initramfs
  and the direct-boot initramfs: doctor current; a disk-image build and a
  direct-boot build succeed with verifying ledgers. (3) Edit a comment in
  `tools/firecracker-vm/init`, `make generate`, rebuild warden: the
  direct-boot initramfs is reported stale and a direct-boot build refused,
  while a disk-image build still succeeds; revert.
- **windows-hyperv** (blocking): (1) with the cache VHDX from main
  (`relay-boot.source` holding a commit, `relay-boot.link-channel`): doctor
  BLOCKED (missing) naming `refresh-cache.sh`; a build is refused before any
  VM. (2) `refresh-cache.sh --check` exits 1 with the reason; a plain run
  rebuilds, writes `relay-boot.vhdx.source`, removes the two old stamps,
  moves the previous disk and stamp to `.prev`; `--check` then exits 0;
  doctor current; an elevated Windows guest build succeeds and its ledger
  verifies. (3) A second plain run with an uncommitted whitespace-only change
  outside the inputs reports current with no rebuild; an uncommitted comment
  edit in `relay/` makes `--check` exit 1; revert. (4)
  `WARDEN_RELAY_BOOT_VHDX` at a copy without its `.source` is refused naming
  the stamp path; with it, accepted. (5) `install-windows.ps1`'s `--check`
  call reports as before.
- **macos-vz** (blocking): (1) with the active image prepared by main
  (`link-channel` only): doctor BLOCKED naming `sudo warden image prepare
  --driver vz`; a build is refused before the VM boots. (2) The refresh in
  place writes `disk.img.source` (no IPSW restore); doctor current; a build
  succeeds and its ledger verifies; `warden-boot.log` matches the previous
  run's lines (the embedded files are byte-identical). (3) `--image` at a
  disk outside the cache with no `.source` is refused naming "omit
  --image". (4) A comment edit in `cmd/warden-io`, `make generate`,
  `make build`: the image is stale; a refresh in place makes it current;
  revert and refresh.
- **macos-qemu** (tracked, backlog `baked-asset-staleness-qemu-windows`): a
  qemu Windows guest build refused on an unstamped relay initramfs and
  succeeding after the rebuild, once `qemu-windows-guest-package` is fixed.

## Implementation plan

Filled in by Plan (phase 2), from the design the review approves. The
outline Plan starts from, each citing its rules:

1. `driver/bootasset` and `digest/`: IDs, input table, digest, stamp API,
   tier-0 tests 1, 2, 4, 5 (Decisions 1, 3, 4; TEST-01, TEST-04, GLOBAL-07).
2. `tools/bootasset`, `make generate`, the generated manifest, test 6
   (Decision 5).
3. The five scripts and `refresh-cache.sh`, test 3 (Decisions 5, 8).
4. `driver.BootAsset` and the per-driver checks and `BootAssets()`, doctor,
   test 7 (Decisions 6, 8, 9; UX-04, ORCH-01).
5. vz guest files to `driver/vz/guest/` with the byte-for-byte golden
   (Decision 9; BOOT-07: `driver/vz/image.go` is on the bootstrap surface,
   and this is not a model change).
6. Retire the link-channel stamp, test 8 (Decision 7; rework of
   build-link-subnet's code, listed as explicit tasks per FLOW-06).
7. CI: `firecracker.yml` cache key and paths, `auto-tidy.yml` (Decision 3).
8. Docs and `overview.md` (UX-01, FLOW-15).

## Design review

None yet.

## Consolidation

Filled in at Publish.

## Rollback notes

None.

## Archive summary

Filled in at Publish.
