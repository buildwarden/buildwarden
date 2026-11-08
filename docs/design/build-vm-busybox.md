---
feature: build-vm-busybox
status: draft
drivers: [qemu, firecracker]  # tools/build-vm builds the qemu direct-boot initramfs; review r1 a2 (C) also re-pins tools/firecracker-vm's busybox to the same version; driver code unchanged
seams: []                # the init, the link-channel stamp and the relay's assets are unchanged
surfaces: []             # the documented setup commands (installation.md, drivers.md) and driver/qemu's fixable message stay correct; only the script's own error text changes
security_boundary: none  # busybox runs inside the build VM, outside the relay boundary; the change adds a pinned checksum where there was none
alters_relationships: false  # the build-vm script still takes the kernel and modules from the relay cache; it only stops taking busybox from it
design_review: required  # FLOW-19: blocking native validation on qemu-any, and two assumed questions
native_validation:
  - {host: qemu-any, blocking: true, reason: "tools/build-vm builds the qemu direct-boot initramfs and nothing else runs it: no CI workflow builds it (firecracker.yml builds tools/firecracker-vm, integration.yml no initramfs at all), and the failure was found on the macbook. The macOS path (sha256 tool, bsdtar on the .apk, an aarch64 ELF the host cannot execute) is exercised only there"}
tier2_gaps: []
rules: [ORCH-01, UX-01, TEST-01, TEST-02, FLOW-01, FLOW-02, FLOW-07, FLOW-09, FLOW-12, FLOW-15, FLOW-19]
rule_changes: []
concepts: [direct-boot, build-vm-initramfs, busybox, alpine-packages, pinned-download, boot-assets, asset-cache, ci-cache-key]
related: [firecracker-driver.md, build-link-subnet.md, unified-bootstrap.md]
---

# tools/build-vm/build-initramfs.sh fetches its own static busybox

## Problem

`tools/build-vm/build-initramfs.sh` builds the qemu direct-boot launcher
initramfs. It copies a static busybox from `tools/relay-vm/.cache/busybox`
and refuses to start without it:

```sh
if [ ! -f "$RELAY_CACHE/busybox" ] || [ ! -f "$RELAY_CACHE/linux-virt.apk" ]; then
    echo "Error: run tools/relay-vm/build-initramfs.sh first" >&2
```

`tools/relay-vm/build-initramfs.sh` no longer writes that file: the relay
initramfs is built from the Alpine minirootfs, which carries its own
(dynamic) busybox. A checkout whose relay cache predates that move still has
the file; a fresh one does not, and following the error's advice does not fix
it. The qemu direct-boot launcher therefore cannot be built from a fresh
cache, and since build-link-subnet (#98) bumped the launcher's link-channel
stamp, every qemu direct-boot user has to rebuild it once.

Found by the macbook native run of 2026-11-05 (digest 2026-11-05, side
finding; repeated in digest 2026-11-06), which placed Alpine's static busybox
in the relay cache by hand to proceed. Queued by Jeff as build-link-subnet's
immediate follow-up (FLOW-09).

## Prior art

- `tools/firecracker-vm/build-initramfs.sh` (firecracker-driver.md,
  Decision 6) already fetches Alpine's `busybox-static` into its own
  `.cache/`, pinned by version and sha256, with an error that tells the
  maintainer how to re-pin when Alpine's branch moves on. This design reuses
  that pattern and its pinned version.
- build-link-subnet.md kept the two direct-boot inits and lease scripts as
  copies, one per `tools/` directory, pinned against drift by tier-0 tests in
  `driver/buildlink/scripts_test.go`, because Firecracker's CI cache key
  hashes `tools/firecracker-vm` and not `tools/build-vm`. The same reasoning
  decides Decision 1 here.
- unified-bootstrap.md: the launcher (init) and its stamp are untouched; a
  different busybox build provides the same applets the init already uses.

## Decisions

### 1. A self-contained pinned fetch in tools/build-vm, its version tied to Firecracker's by a test

The build-vm script fetches `busybox-static-<version>.apk` for **aarch64**
from Alpine 3.21's `main` repository into a new `tools/build-vm/.cache/`,
checks its sha256, and extracts `bin/busybox.static`, as the Firecracker
script does for x86_64.

**Version (review r1, a2 (C)):** both scripts move together to Alpine
3.21's current `busybox-static` now, rather than pinning the build-vm script
to Firecracker's `1.37.0-r14`. Implement reads the current version from the
3.21 `main` index, downloads the aarch64 and x86_64 packages, and pins both
sha256 values. `tools/firecracker-vm/build-initramfs.sh` changes, so
Firecracker's CI asset cache re-keys once and rebuilds on the PR.

Alternatives:

- **A shared helper** (say `tools/lib/alpine-busybox.sh`) used by both
  scripts. Lost: Firecracker's asset cache key in `firecracker.yml` hashes
  `tools/firecracker-vm/build-*.sh`, its configs and init, and its path
  filter lists `tools/firecracker-vm/**`; a helper outside that tree would
  have to be added to both, or a change to it would leave a stale cached
  initramfs. The two fetches also differ in architecture and checksum, so the
  shared part is about ten lines.
- **Restore `busybox` to the relay cache** (have the relay script fetch it
  again). Lost: the relay no longer needs it, so the relay script would be
  fetching for another tool, and its `ARCH=x86_64` cache would gain a file
  nobody reads. It also changes `tools/relay-vm/build-initramfs.sh`, which
  is in Firecracker's cache key and path filter, so it would rebuild
  Firecracker's assets in CI for nothing.
- **Take busybox from the relay's minirootfs.** Lost: it is dynamically
  linked against musl, and the build-vm initramfs ships no libc.

To keep the two pins moving together, a tier-0 test asserts that both
scripts name the same `ALPINE_VERSION` and `BUSYBOX_VERSION` (the checksums
differ per architecture). When Alpine drops the pinned package, both
downloads fail with the same re-pin message and the test makes the re-pin
touch both.

### 2. The relay cache still supplies the kernel and modules

The script keeps reading `linux-virt.apk` from `tools/relay-vm/.cache/`
(the modules must match the relay's `vmlinuz`, which qemu direct boot uses
for the build VM too). Its precondition and error then name only that file.
Fetching the kernel here as well was rejected: it would duplicate the relay
script's APKINDEX lookup and could pick a different kernel from the one in
`tools/relay-vm/output/vmlinuz`.

### 3. Works on the macOS host that runs it

The qemu driver runs only on aarch64 hosts, in practice the macbook
(hosts.yaml), so the fetch must not assume a Linux userland:

- sha256: use `sha256sum -c` when present, else `shasum -a 256 -c` (macOS
  ships `shasum`; the Firecracker script, Linux-only, uses `sha256sum`).
- Applet links: keep the script's explicit applet list. The Firecracker
  script runs `busybox --list-full`, which an x86_64 Linux host can execute
  for its x86_64 busybox, but a macOS host cannot run an aarch64 Linux ELF.
  The explicit list is also what the existing tier-0 test checks for
  `udhcpc`.
- Extraction: `tar -xzf` on the `.apk` with `|| true`, as both existing
  scripts already do (bsdtar and GNU tar each stop after the data segment),
  followed by a check that `bin/busybox.static` was extracted, so a failed
  extraction is reported rather than copied as a missing file.
- Download: `curl -fsSL` to a `.part` file renamed on success, as the
  Firecracker script does, so an interrupted download is not cached.

### 4. No stamp bump

The init, the lease script and the link handling are unchanged, so
`buildlink.Channel` stays as it is and an existing direct-boot initramfs
stays valid. Only a fresh cache, which previously could not build at all,
sees the new path.

## Questions

### Blocking

None.

### Assumed

- **a1 — Where the busybox fetch lives.** Options: (A) a self-contained
  pinned fetch in `tools/build-vm/build-initramfs.sh`, its version tied to
  Firecracker's by a tier-0 test; (B) a shared helper under `tools/`, added
  to `firecracker.yml`'s cache key and path filter; (C) the relay script
  fetches busybox into its cache again. Recommended: (A), Decision 1.
  Proceeding with (A); confirm.
- **a2 — How Implement obtains the aarch64 checksum.** The design session's
  sandbox could not reach `dl-cdn.alpinelinux.org` (proxy refusal), so the
  sha256 of `busybox-static-1.37.0-r14.apk` for aarch64 is not in this
  design. Options: (A) Implement downloads the package and pins its sha256;
  where the cloud sandbox blocks the host, it proposes allowing
  `dl-cdn.alpinelinux.org` through the environment tools (a concrete, blocked
  need), and failing that stops with a finding naming the host; (B) Jeff
  supplies the checksum in the design review, from the package the
  2026-11-05 macbook run fetched or a fresh download, and Implement pins it;
  (C) re-pin both scripts to the branch's current `busybox-static` if r14 is
  gone. Recommended: (A), with (B) as the fallback, and (C) only if the
  pinned version has left Alpine 3.21 (then both scripts move together, which
  re-keys Firecracker's CI cache once). The native step verifies the pin
  either way: a wrong checksum fails its fresh-cache build. Proceeding with
  (A); confirm.

## Design

`tools/build-vm/build-initramfs.sh`, in order:

1. Constants, mirroring the Firecracker script: `ALPINE_VERSION="3.21"`,
   `ALPINE_MIRROR=.../v${ALPINE_VERSION}/main/aarch64`,
   `BUSYBOX_VERSION=<Alpine 3.21's current busybox-static>`, `BUSYBOX_SHA256=<aarch64 sha256>`,
   `BUSYBOX_APK`, `CACHE_DIR="$SCRIPT_DIR/.cache"`, with the same re-pin
   comment.
2. Precondition: `$RELAY_CACHE/linux-virt.apk` present, else "run
   tools/relay-vm/build-initramfs.sh first" (busybox no longer listed).
3. `fetch_busybox`: download to `.part` and rename if not cached, else reuse;
   on a failed download, the Firecracker script's re-pin message; verify the
   sha256 with `sha256sum` or `shasum -a 256` (Decision 3), and stop on a
   mismatch.
4. Extract `bin/busybox.static` into a scratch directory under `.cache/`,
   check it exists, copy it to `$ROOTFS_DIR/bin/busybox`, remove the scratch
   directory.
5. Everything after (applet links, modules, warden-io, init, lease script,
   pack, stamp) unchanged.
6. Header comment: dependencies gain `curl` and `sha256sum` or `shasum`; the
   "Reuses cached artifacts" line says the kernel and modules come from the
   relay cache and busybox is fetched.

`tools/build-vm/.gitignore` (new): `.cache/` and `.rootfs/`, as
`tools/firecracker-vm/.gitignore` has. (`output/` keeps its own ignore
file.)

`driver/buildlink/scripts_test.go`: a test that both direct-boot build
scripts set the same `ALPINE_VERSION` and `BUSYBOX_VERSION`, that each sets a
64-hex `BUSYBOX_SHA256` and checks it, and that the build-vm script no longer
reads `$RELAY_CACHE/busybox`.

No change to `driver/qemu`, the init, the relay script, the reference docs
(the two documented commands are still the two to run, in that order) or
`overview.md` (it has no pseudocode for asset building).

## Cross-driver impact

- **qemu**: its direct-boot initramfs builds from a fresh cache. The
  produced initramfs differs only in the busybox binary; the init's applets
  and the stamp are unchanged. Cloud-image (`--image`) builds do not use it.
- **firecracker**: its script's `BUSYBOX_VERSION` and x86_64
  `BUSYBOX_SHA256` move to the same current version (review r1, a2 (C)).
  Its CI asset cache key hashes `tools/firecracker-vm/build-*.sh`, so the
  cache misses once and `firecracker.yml` rebuilds and runs its builds on
  the PR; that run is its validation.
- **container, vz, hyperv**: unaffected; none uses `tools/build-vm`.

## Validation plan

- **Tier 0** (TEST-02): the new `scripts_test.go` assertions (Design), next
  to the existing direct-boot parity tests.
- **Tier 1**: `make lint` and `make test`. CI: no workflow builds
  `tools/build-vm`. The `driver/buildlink` test change triggers
  `firecracker.yml` and `integration.yml` by their path filters;
  Firecracker's asset cache key changes with its script (a2 (C)), so it
  rebuilds its assets and runs its builds on the new busybox, and `integration.yml` builds no
  initramfs and has no asset cache.
- **Tier 2, native, blocking — `qemu-any`** (step `f6-validate-qemu-any`):
  1. From a fresh cache (`tools/build-vm/.cache`, `tools/relay-vm/.cache`
     and both `output/` directories removed, on a scratch copy so the
     machine's working assets are not lost): `tools/relay-vm/build-initramfs.sh`
     then `tools/build-vm/build-initramfs.sh` succeed with no busybox placed
     by hand; the second run reuses the cached apk.
  2. A corrupted cached apk (one byte changed) makes the script stop on the
     checksum, without producing an initramfs.
  3. Without `tools/relay-vm/.cache/linux-virt.apk`, the script stops with
     the "run tools/relay-vm/build-initramfs.sh first" error.
  4. `warden doctor --driver qemu` accepts the new initramfs's stamp, and a
     direct-boot Linux build on it succeeds with a ledger that verifies.
- No tier-2 gaps.

## Implementation plan

1. Read Alpine 3.21's current `busybox-static` version, download the aarch64
   and x86_64 packages, and pin both checksums; set both scripts to that
   version (review r1 a2 (C)). If the sandbox blocks `dl-cdn.alpinelinux.org`,
   ask to allow it through the environment tools, and failing that stop with
   a finding naming the host. (FLOW-07)
2. Rework `tools/build-vm/build-initramfs.sh` per Design steps 1-6.
   (ORCH-01: qemu only; UX-01: the script's error and header text)
3. Add `tools/build-vm/.gitignore`.
4. Add the tier-0 assertions to `driver/buildlink/scripts_test.go`. (TEST-01,
   TEST-02)
5. `make lint`, `make test`. (FLOW-17)

## Design review

<!-- One entry per review (FLOW-19). -->

- **r1, 2026-11-08: approved at `d196ac9`, read at `9891089`** (Jeff,
  workflow-human, FLOW-19). The design was presented in the chat, with both
  questions in full.
  - **a1: (A)**, a self-contained fetch in `tools/build-vm`, as drafted.
  - **a2: (C)**, re-pin both scripts to Alpine 3.21's current
    `busybox-static` now. This was folded in before the approval (`d196ac9`):
    the Decision 1 version note, Design step 1, the Firecracker
    Cross-driver line, the Validation plan's CI line, and Implementation
    plan task 1.
    - Firecracker's CI asset cache re-keys once.
    - `drivers` adds `firecracker`.
    - Implement downloads both packages to pin their checksums.

## Consolidation

<!-- Filled in at Publish (FLOW-10, FLOW-15). -->

## Rollback notes

## Archive summary

<!-- Filled in at Publish (FLOW-12). -->
