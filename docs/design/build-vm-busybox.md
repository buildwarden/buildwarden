---
feature: build-vm-busybox
status: archived
drivers: [qemu, firecracker]  # tools/build-vm builds the qemu direct-boot initramfs; tools/firecracker-vm is unchanged (its pin was already current, see Consolidation), but driver/buildlink's TestDirectBootBusyboxPins now ties its busybox pin to tools/build-vm's, so its next re-pin must touch both; driver code unchanged
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
sha256 values. Implement found that the current version is still
`1.37.0-r14` for both architectures (the tier-2 review confirmed both
APKINDEXes and checksums), so the build-vm script pins `1.37.0-r14` and
`tools/firecracker-vm/build-initramfs.sh` keeps its pin unchanged; Firecracker's
CI asset cache does not re-key (corrected at Publish, FLOW-10).

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
   `BUSYBOX_VERSION=<Alpine 3.21's current busybox-static>` (`1.37.0-r14`),
   `BUSYBOX_SHA256=<aarch64 sha256>`,
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

## Defaults and overrides

Added at Publish, after #104 made this section required; it restates
Decisions 1-3 and Design step 3 and changes none of them.

- **Default: the pinned package (Decision 1).** The script always fetches
  `busybox-static-1.37.0-r14.apk` for aarch64 from Alpine 3.21's `main`
  repository and checks it against the pinned `BUSYBOX_SHA256`. There is no
  variable, flag or mirror override: the pin is the reproducible choice and
  the one the tier-0 test keeps equal to Firecracker's. Changing it is a
  re-pin of both scripts, not a run-time setting.
- **Selection: `sha256sum`, else `shasum -a 256` (Decision 3).** The script
  checks with `sha256sum -c` when `command -v sha256sum` finds it, and with
  `shasum -a 256 -c` otherwise (macOS, the qemu driver's usual host). It is a
  choice by which tool is present, made before the check runs, not a fallback
  after a failed check: a checksum mismatch under either tool stops the
  script, and the other tool is never tried (GLOBAL-10, GLOBAL-11).
- **Reuse: the cached package (Design step 3).** When
  `tools/build-vm/.cache/busybox-static-<version>.apk` exists, the script
  reuses it instead of downloading, and checks its sha256 on every run, so a
  changed or corrupted cached file stops the script rather than being used.
  An interrupted download is never cached (it is written to `.part` and
  renamed on success). To force a fresh download, delete the cached file.
- **Unchanged source: the relay cache's `linux-virt.apk` (Decision 2).** The
  kernel modules still come only from `tools/relay-vm/.cache/`; when the file
  is missing the script stops and names `tools/relay-vm/build-initramfs.sh`.
  Busybox is never taken from the relay cache any more, even where an old
  cache still holds one.
- **Merges and fallbacks: none else.** No other mirror or Alpine branch is
  tried and a failed download is not retried. `tar -xzf ... || true` is not
  a fallback: it tolerates tar's exit after the apk's data segment, and the
  `bin/busybox.static` check after it stops the script when extraction
  failed (Decision 3).

## Cross-driver impact

- **qemu**: its direct-boot initramfs builds from a fresh cache. The
  produced initramfs differs only in the busybox binary; the init's applets
  and the stamp are unchanged. Cloud-image (`--image`) builds do not use it.
- **firecracker**: unchanged. Its script already pins Alpine 3.21's current
  `busybox-static` (`1.37.0-r14`), so a2 (C)'s re-pin leaves it as it is,
  and its CI asset cache key (which hashes `tools/firecracker-vm/build-*.sh`)
  does not change. The new tier-0 test reads its script, so its next re-pin
  must also re-pin `tools/build-vm`. `firecracker.yml` still runs on the PR
  through its `driver/buildlink/**` path filter, on its cached assets.
- **container, vz, hyperv**: unaffected; none uses `tools/build-vm`.

## Validation plan

- **Tier 0** (TEST-02): the new `scripts_test.go` assertions (Design), next
  to the existing direct-boot parity tests.
- **Tier 1**: `make lint` and `make test`. CI: no workflow builds
  `tools/build-vm`. The `driver/buildlink` test change triggers
  `firecracker.yml` and `integration.yml` by their path filters; Firecracker's
  script is unchanged (its pin is already current), so its asset cache key is
  the same and it runs its builds on the cached assets, and `integration.yml`
  builds no initramfs and has no asset cache. No CI run validates the
  build-vm script; the native step does.
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

1. Pin the version (Decision 1, review r1 a2 (C)): read Alpine 3.21's
   current `busybox-static` version from the `v3.21/main` APKINDEX for
   aarch64 and x86_64, download both `.apk` files and record their sha256.
   If the sandbox blocks `dl-cdn.alpinelinux.org`, propose allowing it
   through the environment tools, and failing that stop with a finding
   naming the host. If the two architectures carry different versions, stop
   with a finding: Decision 1's one shared `BUSYBOX_VERSION` would not hold
   (back to Design). (FLOW-07, ORCH-01)
2. Rework `tools/firecracker-vm/build-initramfs.sh`: set `BUSYBOX_VERSION`
   and the x86_64 `BUSYBOX_SHA256` from task 1; nothing else in the script
   changes (Cross-driver impact, firecracker). (FLOW-06, ORCH-01) Done as a
   no-op: task 1 found `1.37.0-r14`, the version and x86_64 checksum the
   script already pins.
3. Rework `tools/build-vm/build-initramfs.sh` per Design steps 1-6:
   the pinned constants and `CACHE_DIR` with the re-pin comment (aarch64
   sha256 from task 1); the precondition naming only
   `$RELAY_CACHE/linux-virt.apk`; `fetch_busybox` with the `.part` download,
   the re-pin message and the `sha256sum`/`shasum -a 256` check (Decision 3);
   extraction to a scratch directory under `.cache/` with the
   `bin/busybox.static` existence check, replacing the copy from
   `$RELAY_CACHE/busybox`; the explicit applet list and everything after it
   unchanged, with no stamp change (Decision 4); the header's dependencies
   and cache line. (FLOW-06, ORCH-01, UX-01: the script's own error and
   header text)
4. Add `tools/build-vm/.gitignore` with `.cache/` and `.rootfs/`, as
   `tools/firecracker-vm/.gitignore` has. (ORCH-01)
5. Tier 0 (TEST-02): in `driver/buildlink/scripts_test.go`, a pure-Go test
   that reads both direct-boot build scripts and asserts the same
   `ALPINE_VERSION` and `BUSYBOX_VERSION`, a 64-hex `BUSYBOX_SHA256` in each
   that the script checks, and no read of `$RELAY_CACHE/busybox` in the
   build-vm script. The existing stamp, lease-script and applet-list tests
   pass unchanged. (TEST-01, TEST-02)
6. Reference docs and `overview.md`: none owed, per Design. Confirm that
   `installation.md` and `drivers.md` still name the two commands in order
   and `driver/qemu`'s rebuild message still names the build-vm script, and
   change nothing. (UX-01, FLOW-15)
7. Tier 1 gate: `make lint` and `make test` green (no build-tagged files are
   touched, so one platform), then push the branch. On the PR,
   `firecracker.yml` runs through its `driver/buildlink/**` filter on its
   unchanged asset cache; Firecracker owes no new validation, since its script
   did not change. (TEST-01, TEST-02, FLOW-17, ORCH-01)
8. Tier 2, native, blocking (TEST-02, FLOW-09): step `f6-validate-qemu-any`
   runs the Validation plan's four `qemu-any` cases on the pushed branch;
   Implement does not run them.

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

Done at Publish after merging `origin/main` at `4aaeca9` (merge
`3f97023`). The branch forked at `49e5354` (#103).

**Landed on `main` since the fork** (FLOW-10): #104 design-principles
(GLOBAL-09 to GLOBAL-11, RELAY-08, the required **Defaults and overrides**
section), #105 rule-running-line-match, #106 next-skips-merge-wait, #107
wf-squash-after-base-moved, #108 vz-image-prepare-refresh, #109
jks-test-skip-without-jdk. None touches `tools/build-vm`,
`tools/firecracker-vm`, `tools/relay-vm`, `driver/buildlink` or
`driver/qemu`; the merge had no conflicts. #108 changed `drivers.md`,
`installation.md` and `overview.md` around the vz prepared image; the qemu
direct-boot rows and the two documented commands (relay-vm, then build-vm)
are unchanged, so no reference doc is owed (UX-01).

- **Terminology:** aligned. The design and script use the canonical terms
  (direct boot, the launcher, the link-channel stamp, the relay cache).
- **#104's principles:** the **Defaults and overrides** section above is
  added from the approved Decisions without changing one. The tier-2 review
  judged the diff against GLOBAL-09 to GLOBAL-11 and found no unnamed default,
  merge or fallback; RELAY-08 is not reached (no relay setting is delivered).
- **Shared abstractions:** the busybox fetch stays as two copies, one per
  `tools/` directory, by Decision 1 (Firecracker's CI cache key and path
  filter cover only `tools/firecracker-vm`); `TestDirectBootBusyboxPins`
  pins them together.
- **Versioned seams:** none changed (no stamp bump, Decision 4).
- **Convention for future work:** none new. The re-pin rule ("re-pin both
  direct-boot scripts together") is enforced by the tier-0 test, not a rule.
- **Stale design text corrected** (wording only, no decision changed): a2 (C)
  anticipated a version move that did not happen, because Alpine 3.21's
  current `busybox-static` is still `1.37.0-r14`, the version
  `tools/firecracker-vm` already pinned. Corrected: Decision 1's version note,
  Design step 1, the Cross-driver firecracker line, the Validation plan's CI
  line, and Implementation plan tasks 2 and 7. The r1 review record is left
  as written; its "Firecracker's CI asset cache re-keys once" is superseded by
  these corrections. Front matter `drivers` keeps `firecracker`: its script
  is unchanged, but `driver/buildlink`'s `TestDirectBootBusyboxPins` reads it
  and binds its `ALPINE_VERSION` and `BUSYBOX_VERSION` to `tools/build-vm`'s,
  so a future Firecracker re-pin is constrained by this feature.

**FLOW-15 sweep:**

- **Dead code:** `make deadcode` at `3f97023` lists 20 unreachable
  functions, all in Go files this branch does not touch (its only Go change
  is a test); none is introduced here, so none is removed here.
- **Duplication:** the two `fetch_busybox` copies are deliberate (Decision 1,
  above). The tier-2 review noted that Firecracker's copy shares the two
  error-text gaps reported as SHOULD findings; aligning it is left for its
  next re-pin, when its cache key changes anyway.
- **Overview:** `overview.md` has no pseudocode for asset building, and the
  launcher's flow (udhcpc, the stamp check) is unchanged; nothing to update.

## Rollback notes

## Archive summary

**What was built.** `tools/build-vm/build-initramfs.sh`, which builds the
qemu direct-boot launcher initramfs (aarch64), no longer copies busybox from
`tools/relay-vm/.cache/busybox`, a file the relay script stopped producing when
the relay moved to the Alpine minirootfs. It fetches Alpine 3.21's
`busybox-static-1.37.0-r14.apk` for aarch64 into a new `tools/build-vm/.cache/`
(downloaded to `.part`, renamed on success, reused when cached), checks the
pinned sha256 with `sha256sum` or, where that is absent (macOS), `shasum -a
256`, and extracts `bin/busybox.static` with an existence check. The
precondition names only the relay cache's `linux-virt.apk`, which still
supplies the kernel modules. `tools/build-vm/.gitignore` ignores `.cache/` and
`.rootfs/`. `driver/buildlink`'s `TestDirectBootBusyboxPins` requires both
direct-boot scripts to set the same `ALPINE_VERSION` and `BUSYBOX_VERSION`, a
64-hex `BUSYBOX_SHA256` each that the script checks, no read of
`$RELAY_CACHE/busybox`, and the `shasum` branch in the build-vm script. No
stamp bump; driver code, the init, the relay script and the reference docs
are unchanged.

**Decisions taken.** A self-contained pinned fetch per `tools/` directory, tied
by a tier-0 test (a1 (A)); the relay cache keeps supplying the kernel and
modules; a macOS-safe fetch (sha256 tool chosen by presence, the explicit applet
list instead of `busybox --list-full`, a checked extraction); both scripts on
Alpine 3.21's current `busybox-static` (a2 (C)), which turned out to be the
`1.37.0-r14` Firecracker already pinned, so `tools/firecracker-vm` did not
change and its CI asset cache did not re-key.

**Rejected.** A shared helper under `tools/` (Firecracker's cache key and path
filter would have to grow, for about ten shared lines); restoring busybox to
the relay cache (the relay no longer needs it, and its script is in
Firecracker's cache key); the minirootfs busybox (dynamically linked, and the
launcher initramfs has no libc); fetching the kernel in the build-vm script
too (it could diverge from the relay's `vmlinuz`).

**For a future design touching these concepts.** The two direct-boot busybox
fetches are deliberate copies: a re-pin (Alpine keeps only a branch's current
package, so the download fails when it moves) changes both scripts in one
change, with each architecture's checksum, and Firecracker's asset cache then
re-keys. The build-vm script runs only on the qemu driver's aarch64 host, in
practice macOS, and no CI workflow builds it, so a change to it is validated
natively (`qemu-any`). The two error texts it shares with the Firecracker copy
(a checksum mismatch reports only the tool's output; any download failure
reads as "re-pin") were tier-2 SHOULD findings left open at merge.
