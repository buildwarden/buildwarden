---
feature: vz-image-prepare-refresh
status: draft
drivers: [vz]            # driver/vz (image.go, driver.go) and the vz path of cmd/warden/image.go only; no shared driver/ package changes, so no other driver is reached (FLOW-02)
seams: []                # no versioned seam: the active-image pointer is the vz image cache's own layout, read only by driver/vz
surfaces: [cli, output]  # cli: `image prepare --driver vz` with no source refreshes in place, the `latest` source, a prepared image directory as source, `image list` lists every vz image; output: the refresh and newer-IPSW notices, the stamp refusal's fix text (UX-01, UX-04)
security_boundary: none  # which prepared image a build boots changes; the image's isolation, the relay path and what is baked into it do not
alters_relationships: false  # inside the vz driver's existing role: how it picks and refreshes its own base image
design_review: required  # blocking native validation (macos-vz) and assumed questions (FLOW-19)
native_validation:
  - {host: macos-vz, blocking: true, reason: "reaches driver/vz only (image selection, refresh in place, prepare's source handling) and alters behavior only the vz host exercises: the IPSW restore, the in-place re-prepare and the build's choice among prepared images cannot run in CI"}
tier2_gaps: []
rules: [FLOW-01, FLOW-02, FLOW-09, FLOW-12, FLOW-15, FLOW-19, UX-01, UX-03, UX-04, UX-05, UX-07, GLOBAL-04, GLOBAL-05, GLOBAL-07, ORCH-01, TEST-02]
rule_changes: []
concepts: [image-prepare, image-refresh, image-cache, active-image, ipsw, vz, launcher-stamp, multi-version-images]
related: [build-link-subnet.md, hyperv-windows-prepare.md, vm-driver-release-plan.md, ledger-environment-types.md]
---

# vz image prepare refreshes the cached image in place, and builds pick the prepared image

## Problem

Two defects in the vz driver's image cache, both on `main` before
build-link-subnet (#98) and made visible by it. Found in the attended macos-vz
validation of build-link-subnet (workflow branch
`native/macos-vz/2026-11-05/evidence.md`, "Re-prepare"; PR #98 "For Jeff"
item 4):

1. **`sudo warden image prepare --driver vz` with no source does not refresh
   the prepared image.** `prepareVZMacos` always calls
   `ImageCache.RestoreIPSW("")`, which resolves the default source to the
   *latest supported IPSW* and keys the image directory by it
   (`macos-<shortHash(url)>`, beside `ipsw-<same>.ipsw`). The in-place refresh
   (`ReprepareImage`) runs only when that directory is already prepared *and*
   is the one `ImageDir` returns. Once Apple publishes a newer macOS (27.2,
   26B123 here), a "refresh" becomes a 25.6 GB download, a restore and a first
   boot (~13 min) into a new `macos-a139c218a2010753`, leaving
   `macos-90a1b1f94f64b00a` (27.0.1) untouched. The guide
   (`docs/book/src/guide/drivers.md`, First-Time Setup and the build-link
   table) and build-link-subnet Decision 6 both promise "in place, no IPSW
   restore"; that holds only while the latest IPSW is unchanged.
2. **A build without `--image` uses the first prepared directory by name.**
   `ImageCache.ImageDir` returns the first valid directory in `os.ReadDir`
   order. With both images present it keeps returning `macos-90a1…`, the old
   unstamped image, which build-link-subnet's link-channel stamp check refuses
   with `sudo warden image prepare --driver vz` as the fix, and that fix does
   not touch `macos-90a1…`. The only way through was `--image <dir>/disk.img`.
   The same defect means a refresh with an explicit IPSW source whose
   directory sorts second never re-prepares it either (the `existing ==
   filepath.Dir(path)` test in `prepareVZMacos` fails).

`doctor --driver vz` and `image list` inherit (2): they report the
first-by-name image, not the one the user last prepared.

Why now: it is an immediate follow-up of build-link-subnet (FLOW-09), whose
stamp refusal names a fix that does not work on any machine that has seen a
macOS release since its image was prepared. The queued `baked-asset-staleness`
will name the same command for a stale vz image, so it depends on this too.

## Prior art

- **build-link-subnet** (`docs/design/build-link-subnet.md`, Decision 6): the
  launcher stamp and its fix for vz, "`sudo warden image prepare --driver vz`,
  which re-prepares an existing image in place (`ReprepareImage`; no IPSW
  restore)", and the `--image` outside-the-cache fix (Review r1 SHOULD 3).
  This design makes that statement true and keeps both fixes; it changes only
  which image the first one refreshes and adds one for a cache image that is
  not active.
- **hyperv-windows-prepare** (`docs/design/hyperv-windows-prepare.md`,
  concepts `image-prepare`, `image-refresh`): Hyper-V's `image prepare` with no
  source is the refresh, an active base plus older ones listed by `image list`,
  and `[vm] windows_image_update = "auto"` picking up new media at refresh.
  Reused: "no source is the refresh", an explicit active image, `image list`
  showing active and older. Departed from: vz does not pick up a newer IPSW at
  refresh (Decision 3), and does not delete older images (Decision 6).
- **vm-driver-release-plan** (`docs/design/vm-driver-release-plan.md`, "Image
  Lifecycle Management"): names updates and multi-version support (macOS 14
  and 15 side by side) as open needs, and sketches `image = "latest"  # or a
  path, or an IPSW URL`. Reused: the `latest` spelling (Decision 3) and keeping
  several versions (Decision 6).
- **ledger-environment-types** (`docs/design/ledger-environment-types.md`):
  `recordPreparedImage` records the IPSW URL and digest as the prepared disk's
  preparation input in the layer-hash cache. Reused unchanged; `image list`
  reads the IPSW name from it (Decision 5).
- Rules: UX-04 (a refusal names a fix that works), UX-01 (the guide, help text
  and `overview.md` change with the behavior), UX-03 (additive: `latest` and a
  directory source are new source forms; nothing is removed), UX-07 (a
  predictable default), GLOBAL-05 (the build OS does not change silently
  between builds), ORCH-01 (vz only).

All four designs already carry front-matter; none needs a retrofit (FLOW-12).

## Decisions

1. **The cache records an active image.** Every successful `warden image
   prepare --driver vz` (restore or refresh) ends by writing `<cache>/active`,
   one line naming the image directory (`macos-<key>`), and handing it back to
   the sudo user like the rest of the cache. `ImageCache.ImageDir` (builds
   without `--image`, `doctor`, the stamp list) returns the active directory
   when it is a valid prepared image. Otherwise (no pointer, as on every cache
   prepared before this change, or a pointer to a removed or broken directory)
   it falls back to the valid prepared directory whose `.prepared` marker is
   newest, the name breaking ties, so the result is deterministic and is the
   image the user prepared last.
   *Rejected:* newest `.prepared` alone (implicit: any touch of a marker
   changes the build OS, and a refresh of an older image would silently make
   it current; it stays only as the fallback); refusing to build when more than
   one image exists without `--image` (breaks every multi-version cache for a
   choice the user already made by preparing, UX-07); a symlink (the same
   information, harder to hand back to the sudo user and to validate).

2. **No source means refresh the active image in place.** With no source and
   an active image (Decision 1, fallback included), `image prepare --driver vz`
   does not resolve or download an IPSW: it runs `ReprepareImage` on that
   directory (boot agent, `warden-io`, provenance, stamp, `.prepared`) and
   writes the pointer. With no prepared image at all it resolves the latest
   supported IPSW and restores it, as first-time setup does today. This is what
   the guide and build-link-subnet already promise.
   *Rejected:* keep resolving the latest IPSW at every refresh (the defect); a
   refresh that refreshes the active image and also restores the latest when it
   differs (two 25 GB costs the user did not ask for in one command).

3. **A newer macOS only on request: the `latest` source.** `warden image
   prepare --driver vz latest` (or `--source latest`) resolves the latest
   supported IPSW and restores it, or refreshes its directory in place when it
   is already prepared, and makes it active. A refresh with no source makes one
   best-effort lookup of the latest IPSW after it succeeds; when no prepared
   directory exists for it, it prints a notice naming the version and `sudo
   warden image prepare --driver vz latest` (about 25 GB, 15 minutes). A
   failed lookup (offline, Apple unreachable) prints nothing and never fails
   the refresh.
   *Rejected:* reusing `--force` ("re-acquire even if a valid cached copy
   exists": for vz that would mean re-restoring the same IPSW, a different and
   destructive meaning); a new `--update` flag (a second spelling for what a
   source already expresses; qemu's `ubuntu:24.04` sources are symbolic the
   same way); `[vm] macos_image_update = "auto"` mirroring Hyper-V (a new macOS
   is a different build OS, so picking it up at refresh changes builds
   silently, GLOBAL-05; Hyper-V's `auto` exists because re-issued evaluation
   media of the *same* OS is how its licence clock is reset, which vz has no
   counterpart for).

4. **A source that names a prepared image refreshes it in place.** The source
   is classified in one pure function: empty, `latest`, a URL, a local IPSW
   file, or a prepared image (a cache directory that `isValidImageDir`
   accepts, or the `disk.img` inside one). An IPSW (URL or file, or `latest`
   resolved) whose `macos-<key>` directory is already prepared is refreshed in
   place, whichever position it sorts in, which fixes the second half of
   problem (2). A prepared-image source is refreshed in place. Every path ends
   with that image active. A prepared-image source outside the cache is
   refused with the existing fix (`customImageFix`): prepare only re-prepares
   what it owns.
   *Rejected:* refresh a named image without making it active (then the
   command the stamp refusal names would leave builds without `--image` on a
   different image; "the image you prepared last is the one builds use" is
   the single rule to remember).

5. **Fix text and listings follow the active image.** The stamp refusal (build
   and `doctor`) names: for the active image, `sudo warden image prepare
   --driver vz` (unchanged text, now correct); for a cache image that is not
   active, given with `--image`, `sudo warden image prepare --driver vz
   <dir>` (refreshes it and makes it active) or "omit --image to build from the
   active image"; for a disk outside the cache, `customImageFix` unchanged.
   `image list` prints every prepared vz image, the active one marked, each
   with its IPSW file name from the recorded provenance when known (for
   example `UniversalMac_27.2_26B123_Restore.ipsw`).
   *Rejected:* listing only the active image (hides the 25 GB+ images the
   user may want to remove or switch to).

6. **Older images stay; reclaiming disk is a follow-up.** No image or IPSW is
   deleted by prepare. Keeping several macOS versions is a stated need
   (vm-driver-release-plan), and an image directory plus its IPSW is the
   user's only rollback. `image list` makes them visible (Decision 5). A
   retention policy (Hyper-V deletes layers older than the active one and the
   one it replaced) is a separate item, proposed as `vz-image-retention`, for
   Jeff to queue or not.

7. **Out of scope, noted.** `ensureIPSWDownloaded` prefers a pre-existing
   `ipsw-latest.ipsw` over downloading *any* URL, so a stale manual file could
   be restored into a directory keyed by a newer URL (the recorded digest stays
   honest). `resolveImage` restores an IPSW for `build --image <url>`, which
   its own comment says builds never do. Neither is touched here; a build
   restore does not write the pointer.

## Questions

### Blocking

None.

### Assumed

- **a1. The default of `image prepare --driver vz` with no source** (Decision
  2). Proceeding with (a); confirm. Options: (a) refresh the active image in
  place, no IPSW lookup; restore the latest only when no image is prepared;
  (b) as Hyper-V: pick up a newer IPSW at refresh by default, with a config key
  to keep the current one; (c) refresh the active image and also restore the
  latest when it differs. Recommended: (a).
- **a2. How the user asks for a newer macOS** (Decision 3). Proceeding with
  (a); confirm. Options: (a) the source keyword `latest` (positional or
  `--source`), plus a notice after a refresh when a newer IPSW has no prepared
  image; (b) `--force`; (c) a new `--update` flag. Recommended: (a).
- **a3. Which image a build without `--image` uses** (Decision 1). Proceeding
  with (a); confirm. Options: (a) an active pointer written by every prepare,
  falling back to the newest `.prepared` marker; (b) the newest `.prepared`
  marker only; (c) refuse when more than one image is prepared and `--image` is
  absent. Recommended: (a).
- **a4. A prepared image named as the source** (Decision 4). Proceeding with
  (a); confirm. Options: (a) accepted (directory or its `disk.img`, inside the
  cache), refreshed in place and made active; (b) accepted, refreshed, active
  unchanged; (c) not accepted (IPSW sources only). Recommended: (a).
- **a5. Older images and IPSWs** (Decision 6). Proceeding with (a); confirm.
  Options: (a) kept and listed; retention is a follow-up item
  (`vz-image-retention`); (b) after a successful restore, delete images older
  than the active one and the one it replaced, with their IPSWs (Hyper-V's
  rule); (c) delete only the IPSW once its image is prepared. Recommended: (a).

## Design

### Image cache

```
<cache> = ~/.cache/warden/images  (the sudo user's under sudo)
  active                      "macos-<key>\n"          new (Decision 1)
  ipsw-<key>.ipsw             <key> = shortHash(IPSW URL or local path)
  macos-<key>/                disk.img aux-storage hardware-model machine-id
                              .prepared link-channel
  layers.json                 provenance (IPSW URL, digest) per prepared disk
```

`driver/vz/image.go` gains, on `ImageCache`:

```
Images() []Image                    every valid prepared dir: {Dir, Prepared time, Active bool}
ImageDir() (string, error)          active if valid, else newest .prepared (name tie-break), else ""
SetActive(dir) error                write <cache>/active (atomic rename), owned by the sudo user
Prepare(source, latest func) (dir, error)    the decision below; cmd/warden calls only this
```

`latest` is injected (`latestSupportedIPSW` on darwin/arm64), so the decision
is tier-0 testable on every OS (GLOBAL-07).

### `image prepare --driver vz [source]`

```
preflight; require root                               (unchanged, before anything)
kind := classify(source)        empty | latest | url | ipsw-file | prepared-dir
switch:
  empty, active image A exists  -> refresh(A)
  empty, none prepared          -> restore(latestSupportedIPSW())
  latest                        -> u := latestSupportedIPSW(); restoreOrRefresh(u)
  url | ipsw-file               -> restoreOrRefresh(source)
  prepared-dir D in cache       -> refresh(D)
  prepared-dir outside cache    -> Fixable(customImageFix)
SetActive(result)
empty-source refresh only: best effort u := latestSupportedIPSW();
  no prepared macos-<shortHash(u)>  -> notice "macOS <name> is available: sudo warden image prepare --driver vz latest (~25 GB)"
  lookup error                      -> nothing

restoreOrRefresh(u): dir := macos-<shortHash(u)>
  valid prepared -> refresh(dir)            (any position in the cache)
  else           -> RestoreIPSW(u)          (unchanged: download, restore, headless, first boot, provenance, stamp)
refresh(dir) = ReprepareImage(dir)          (unchanged)
```

### Build, doctor, list

```
build --driver vz:
  --image given  -> that disk (unchanged)
  else           -> ImageCache.ImageDir()  (active, else newest prepared)
  stamp check:   active image        -> fix PrepareCommand
                 cache, not active   -> fix "sudo warden image prepare --driver vz <dir>, or omit --image"
                 outside the cache   -> customImageFix (unchanged)
doctor --driver vz: the active image's stamp (via ImageDir)
image list:  vz  macos  <dir>  active|older  <IPSW file name from provenance>
```

### Docs (UX-01, FLOW-15)

- `docs/book/src/guide/drivers.md`: VZ First-Time Setup (no source refreshes
  the active image in place; `latest` moves to a new macOS; builds use the
  image prepared last; `--image` and a directory source pick another), the
  build-link table row and the paragraph after it.
- `cmd/warden/image.go`: the `image prepare` help (`--driver vz` line,
  Example).
- `docs/book/src/concepts/overview.md`, VM drivers: the stamp line's vz fix,
  and one line for the vz base image choice (`--image`, else the cache's
  active image).
- `docs/book/src/guide/cli.md`: `warden image prepare` (the vz row of the
  source table, the examples) and `warden image list` (the example output,
  which also shows a stale `vz/images/macOS-15.4` path today).

## Cross-driver impact

- **vz:** everything above; `driver/vz/image.go`, `driver/vz/driver.go`
  (`resolveImage`, `imageStamp`, `LauncherStamps`) and the vz branches of
  `cmd/warden/image.go` (`prepareVZMacos`, `runImageList`).
- **qemu, firecracker, hyperv, container:** unaffected. No shared package
  changes; `cmd/warden/image.go`'s other branches and `imageSource` are
  untouched, and `latest` is classified only on the vz path (ORCH-01). Hyper-V
  already has an explicit active base.

## Validation plan

- **Tier 0 (every OS, `make test`):** `classify` for each source form;
  `ImageDir` with a pointer, without one (newest `.prepared`, name tie-break),
  with a pointer to a missing or invalid dir; the prepare decision with a
  stubbed `latest` (each row of the switch, including an IPSW whose dir sorts
  second, and that no lookup happens for an empty-source refresh before it
  succeeds); the notice on and off, and a lookup error staying silent;
  `imageStamp`'s three fix texts; `image list` formatting from a fake cache
  (GLOBAL-04).
- **Tier 2, native, blocking: macos-vz** (FLOW-09). Run on Jeff's
  Apple-silicon Mac, whose cache still holds `macos-90a1b1f94f64b00a`
  (27.0.1, unstamped), `macos-a139c218a2010753` (27.2, stamped) and both
  IPSWs, with no `active` file, so the pre-change cache is the starting point:
  1. A build with no `--image` uses `macos-a139…` (newest `.prepared`);
     `image list` marks it active and lists `macos-90a1…` as older with its
     IPSW name; `doctor --driver vz` reports `macos-a139…`'s stamp.
  2. `sudo warden image prepare --driver vz` with no source refreshes
     `macos-a139…` in place: no IPSW download or restore, "Refreshing boot
     agent", `active` written and owned by the user; then a build with no
     `--image` succeeds and its ledger verifies.
  3. `sudo warden image prepare --driver vz ~/.cache/warden/images/macos-90a1…`
     refreshes the old image in place (no restore), stamps it and makes it
     active; a build with no `--image` boots 27.0.1 and its ledger verifies.
  4. With `macos-90a1…` active, `--image ~/.cache/warden/images/macos-a139…/disk.img`
     after removing that image's `link-channel` is refused naming `sudo warden
     image prepare --driver vz <macos-a139… dir>`; running it fixes the build
     and makes `macos-a139…` active.
  5. `sudo warden image prepare --driver vz latest` (27.2 already prepared)
     refreshes `macos-a139…` in place with no download.
  6. With the Mac offline, `sudo warden image prepare --driver vz` still
     refreshes the active image and prints no newer-IPSW notice or error.
- **Tier-2 gaps:** none. The newer-IPSW notice's positive case (a release
  newer than every prepared image) cannot be staged on demand; it is covered
  at tier 0 with the stubbed lookup.

## Implementation plan

Filled in at Plan (phase 2), from the approved design.

## Design review

## Consolidation

## Rollback notes

## Archive summary
