---
feature: vz-image-prepare-refresh
status: archived
drivers: [vz]            # driver/vz (image.go, driver.go, environment.go) and the vz path of cmd/warden/image.go only; no shared driver/ package changes, so no other driver is reached (FLOW-02)
seams: []                # no versioned seam: the active-image pointer is the vz image cache's own layout, read only by driver/vz
surfaces: [cli, output]  # cli: `image prepare --driver vz` with no source refreshes in place, the vz `--update` flag, a prepared image directory as source, `image list` lists every vz image, a local IPSW path is restored from in place; output: the refresh and out-of-date warnings, the stamp refusal's fix text (UX-01, UX-04)
security_boundary: none  # which prepared image a build boots changes; the image's isolation, the relay path and what is baked into it do not
alters_relationships: false  # inside the vz driver's existing role: how it picks and refreshes its own base image
design_review: required  # blocking native validation (macos-vz), and the delta since approved at f764e9f changes decisions (return r2: warning condition, local IPSW restore) (FLOW-19)
native_validation:
  - {host: macos-vz, blocking: true, reason: "reaches driver/vz only (image selection, refresh in place, prepare's source handling) and alters behavior only the vz host exercises: the IPSW restore, the in-place re-prepare and the build's choice among prepared images cannot run in CI"}
tier2_gaps: []
rules: [FLOW-01, FLOW-02, FLOW-09, FLOW-12, FLOW-15, FLOW-19, UX-01, UX-03, UX-04, UX-05, UX-07, GLOBAL-04, GLOBAL-05, GLOBAL-07, ORCH-01, TEST-02]
rule_changes: []
concepts: [image-prepare, image-refresh, image-update, image-cache, active-image, ipsw, local-ipsw-source, vz, launcher-stamp, multi-version-images]
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

3. **A local IPSW path as the source fails.** The `image prepare` help
   ("local path or URL") and `docs/book/src/guide/cli.md` promise a local
   path, but `RestoreIPSW` hands every source to curl (`ensureIPSWDownloaded`),
   which rejects a plain path with "URL rejected: No host part"; only a
   `file://` URL works, and it is copied into the cache. Found by Plan (return
   r2, Design review).

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
  path, or an IPSW URL`. Reused: keeping several versions (Decision 6); the
  `latest` spelling was proposed and replaced by `--update` at design review
  (Decision 3).
- **ledger-environment-types** (`docs/design/ledger-environment-types.md`):
  `recordPreparedImage` records the IPSW URL and digest as the prepared disk's
  preparation input in the layer-hash cache. Reused unchanged; `image list`
  reads the IPSW name from it (Decision 5).
- Rules: UX-04 (a refusal names a fix that works), UX-01 (the guide, help text
  and `overview.md` change with the behavior), UX-03 (additive: `--update` and
  a directory source are new; nothing is removed), UX-07 (a
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

3. **A newer macOS only on request: `--update`.** `warden image prepare
   --driver vz --update` resolves the latest supported IPSW and restores it,
   or refreshes its directory in place when it is already prepared, and makes
   it active. It takes no source: with a positional or `--source` source it is
   refused ("--update picks the newest IPSW itself; drop the source, or drop
   --update to prepare that source"). On any other driver it is refused with
   "--update is not supported for --driver <d> yet" (cross-driver `--update` is
   a separate queued item, Design review). A refresh with no source makes one
   best-effort lookup of the latest supported IPSW `u` after it succeeds, and
   ends with a **warning** whenever the active image was not restored from `u`,
   **whether or not `u` already has a prepared image** (Jeff, return r2). The
   warning names both versions and `--update`, which restores `u` or, when its
   image is already prepared, refreshes that image in place and makes it
   active with no download:
   - `u` not prepared: `warning: the active macOS image is <current>; macOS
     <newer> is available. To update (about 25 GB, 15 minutes): sudo warden
     image prepare --driver vz --update`
   - `u` prepared (`macos-<shortHash(u)>` valid): `warning: the active macOS
     image is <current>; macOS <newer> is already prepared. To use it (no
     download): sudo warden image prepare --driver vz --update`

   A failed lookup (offline, Apple unreachable) prints nothing and never fails
   the refresh.

   **The active image's IPSW identity.** The comparison is well defined from
   what the cache already holds. An image restored from IPSW source `s` is the
   directory `macos-<shortHash(s)>` (the key), and its prepared disk's recorded
   provenance (`layers.json`, ledger-environment-types) carries the input
   `{name: ipsw, url, sha256, size}`. The active image *is* from `u` when
   either (a) its directory is `macos-<shortHash(u)>`, or (b) the file name of
   its recorded IPSW `url` (the last path element) equals the file name of `u`.
   (b) is what makes an image prepared from a local file comparable: its key
   hashes the path, never `u`, but Apple's file name
   (`UniversalMac_<version>_<build>_Restore.ipsw`) survives a download kept
   under its own name. A local copy that was renamed does not match and gets
   the warning; `--update` then restores `u` from Apple, which is what the
   warning says it does. `<current>` and `<newer>` are the version and build
   parsed from those file names (`macOS 27.2 (26B123)`), else the file name;
   an active image with no recorded IPSW (prepared before provenance was
   recorded) and a key that is not `u`'s is named by its directory
   (`macos-<key> (IPSW not recorded)`) and gets the warning. The digest is not
   compared: knowing `u`'s digest needs the 25 GB download the warning is
   there to offer.
   *Rejected:* warning only when `u` has no prepared image (the r1 text; it
   contradicted native case 3, and an older active image is out of date
   whether or not the newer one is on disk, Jeff at return r2); a `latest`
   source keyword (proposed by Design; Jeff chose a
   flag at review: a source names *what* to prepare, `--update` asks to move
   on, and the flag is the spelling other drivers can share later); reusing
   `--force` ("re-acquire even if a valid cached copy exists": for vz that
   would mean re-restoring the same IPSW, a different and destructive
   meaning); `[vm] macos_image_update = "auto"` mirroring Hyper-V (a new macOS
   is a different build OS, so picking it up at refresh changes builds
   silently, GLOBAL-05; Hyper-V's `auto` exists because re-issued evaluation
   media of the *same* OS is how its licence clock is reset, which vz has no
   counterpart for).

4. **A source that names a prepared image refreshes it in place.** The source
   is classified in one pure function: empty, a URL (has a scheme, `file://`
   included), a local IPSW file (an existing regular file given as a plain
   path, Decision 7), or a prepared image (a cache directory that
   `isValidImageDir` accepts, or the `disk.img` inside one); a plain path that
   names none of these is refused naming the path ("no IPSW file or prepared
   image at <path>; give an IPSW URL, a local .ipsw file, or a prepared image
   directory", UX-04). An IPSW (URL or file, or the one `--update`
   resolves) whose `macos-<key>` directory is already prepared is refreshed in
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

6. **Older images stay; an image's IPSW is deleted once it is prepared.**
   No image directory is deleted by prepare: keeping several macOS versions is
   a stated need (vm-driver-release-plan), an image is the user's only
   rollback, and `image list` makes them visible (Decision 5). Each successful
   prepare of an image (restore or refresh) ends by deleting that image's own
   `ipsw-<key>.ipsw` (about 25 GB), after the provenance (URL, digest) is
   recorded, so the IPSW is never needed again: a later prepare of the same
   source finds the prepared directory and refreshes it in place
   (Decision 4), and only a restore downloads. A local IPSW file given as the
   source is the user's and is never deleted; only the cache's
   `ipsw-<key>.ipsw` copy is. The IPSWs of other images are left until their
   own image is next prepared. The cost: re-restoring an image from scratch
   (after deleting its directory) downloads its IPSW again.
   *Rejected:* keeping every IPSW until a retention item (about 25 GB each for
   a file nothing reads again); Hyper-V's rule of deleting images older than
   the active one and the one it replaced (removes rollback images without
   the user asking).

7. **A local IPSW file is restored from in place** (Jeff, return r2; fixes
   problem 3). For a plain-path source, `RestoreIPSW` reads the file where it
   is: no curl, no copy into the cache, no `ipsw-<key>.ipsw` written, and the
   file is never deleted (Decision 6). The image is keyed by its path as for
   any source, `macos-<shortHash(path)>`, with a relative path made absolute
   first so the same file keys the same from any directory. Its provenance
   records the file's digest and size, and `url: file://<absolute path>` so
   that the input stays a URL and its file name is known for `image list` and
   the out-of-date comparison (Decision 3). A `file://` URL source is
   unchanged (a URL: curl copies it into the cache, and that copy is deleted
   once prepared, Decision 6), so its key does not change for an existing
   image. A second prepare of the same path finds the prepared directory and
   refreshes it in place (Decision 4).
   *Rejected:* refuse a plain path and require `file://` (breaks what the help
   and `cli.md` already promise, UX-03); copy the file into the cache like a
   download (25 GB of copying and disk for a file the user already has).

8. **Out of scope, noted.** `ensureIPSWDownloaded` prefers a pre-existing
   `ipsw-latest.ipsw` over downloading *any* URL, so a stale manual file could
   be restored into a directory keyed by a newer URL (the recorded digest stays
   honest); a local-file source bypasses it (Decision 7). `resolveImage`
   restores an IPSW for `build --image <url>`, which its own comment says
   builds never do. Neither is touched here; a build
   restore does not write the pointer.

## Questions

### Blocking

None.

### Assumed

- **a1. The default of `image prepare --driver vz` with no source** (Decision
  2). Options: (a) refresh the active image in place, no IPSW lookup; restore
  the latest only when no image is prepared; (b) as Hyper-V: pick up a newer
  IPSW at refresh by default, with a config key to keep the current one; (c)
  refresh the active image and also restore the latest when it differs.
  Recommended: (a). **Answered at review (Jeff):** (a), and end the refresh
  with a warning that the image is out of date, giving the command to update
  it (Decision 3).
- **a2. How the user asks for a newer macOS** (Decision 3). Options: (a) the
  source keyword `latest` (positional or `--source`), plus a notice after a
  refresh when a newer IPSW has no prepared image; (b) `--force`; (c) a new
  `--update` flag. Recommended: (a). **Answered at review (Jeff):** (c), a new
  `--update` flag, vz only in this feature; `--update` for the other drivers is
  a separate queued item.
- **a3. Which image a build without `--image` uses** (Decision 1). Options:
  (a) an active pointer written by every prepare, falling back to the newest
  `.prepared` marker; (b) the newest `.prepared` marker only; (c) refuse when
  more than one image is prepared and `--image` is absent. Recommended: (a).
  **Answered at review (Jeff):** (a).
- **a4. A prepared image named as the source** (Decision 4). Options: (a)
  accepted (directory or its `disk.img`, inside the cache), refreshed in place
  and made active; (b) accepted, refreshed, active unchanged; (c) not accepted
  (IPSW sources only). Recommended: (a). **Answered at review (Jeff):** (a).
- **a5. Older images and IPSWs** (Decision 6). Options: (a) kept and listed;
  retention is a follow-up item (`vz-image-retention`); (b) after a successful
  restore, delete images older than the active one and the one it replaced,
  with their IPSWs (Hyper-V's rule); (c) delete only the IPSW once its image is
  prepared. Recommended: (a). **Answered at review (Jeff):** (c).

## Design

### Image cache

```
<cache> = ~/.cache/warden/images  (the sudo user's under sudo)
  active                      "macos-<key>\n"          new (Decision 1)
  ipsw-<key>.ipsw             downloaded IPSW (URL sources only); deleted once prepared (Decision 6)
  macos-<key>/                <key> = shortHash(IPSW URL, or a local file's absolute path)
                              disk.img aux-storage hardware-model machine-id
                              .prepared link-channel
  layers.json                 provenance per prepared disk: input {name: ipsw, url, sha256, size};
                              url is the IPSW URL, or file://<absolute path> for a local file
```

`driver/vz/image.go` gains, on `ImageCache`:

```
Images() []Image                    every valid prepared dir: {Dir, Prepared time, Active bool, IPSW name}
ImageDir() (string, error)          active if valid, else newest .prepared (name tie-break), else ""
SetActive(dir) error                write <cache>/active (atomic rename), owned by the sudo user
Prepare(source, update) (dir, error)                 the decision below; cmd/warden calls only this
OutOfDate(active, u) (warning string, ok bool)       Decision 3's comparison and text; pure
```

The host steps are seams on `ImageCache`, nil meaning the real one: the
lookup (`latest`, `latestSupportedIPSW` on darwin/arm64), the download, the
restore step that turns the acquired IPSW into a disk (`restoreDisk`), the
in-place refresh and the root check, so the
decision, the local-file path and the warning are tier-0 testable on every OS
(GLOBAL-07).

### `image prepare --driver vz [source | --update]`

```
preflight; require root                               (unchanged, before anything)
--update with a source         -> refuse (drop one or the other)
kind := classify(source)        empty | url | ipsw-file | prepared-dir | none-of-these -> refuse naming the path
switch:
  --update                      -> u := latestSupportedIPSW(); restoreOrRefresh(u)
  empty, active image A exists  -> refresh(A)
  empty, none prepared          -> restore(latestSupportedIPSW())
  url | ipsw-file               -> restoreOrRefresh(source)
  prepared-dir D in cache       -> refresh(D)
  prepared-dir outside cache    -> Fixable(customImageFix)
SetActive(result)
delete <cache>/ipsw-<key of result>.ipsw if present  (provenance already recorded; a user's local IPSW is never touched)
empty-source refresh only: best effort u := latestSupportedIPSW();
  lookup error                                   -> nothing
  active is from u                               -> nothing
      (dir == macos-<shortHash(u)>, or base(recorded ipsw url) == base(u))
  not from u, macos-<shortHash(u)> not prepared  -> warning "the active macOS image is <current>; macOS <newer> is available. To update (about 25 GB, 15 minutes): sudo warden image prepare --driver vz --update"
  not from u, macos-<shortHash(u)> prepared      -> warning "the active macOS image is <current>; macOS <newer> is already prepared. To use it (no download): sudo warden image prepare --driver vz --update"
  <current>/<newer>: version (build) parsed from the IPSW file name, else the file name;
                     no recorded IPSW -> "macos-<key> (IPSW not recorded)"
--update on any other driver   -> refuse "--update is not supported for --driver <d> yet", fix "drop --update (only --driver vz supports it so far)"

restoreOrRefresh(s): s := abs(s) for an ipsw-file; dir := macos-<shortHash(s)>
  valid prepared -> refresh(dir)            (any position in the cache)
  else           -> RestoreIPSW(s)          (restore, headless, first boot, provenance, stamp: unchanged)
RestoreIPSW(s) acquires the IPSW by kind:
  url        -> ensureIPSWDownloaded (curl into <cache>/ipsw-<key>.ipsw, unchanged); provenance url = s
  ipsw-file  -> s itself, read in place: no curl, no copy, never deleted; provenance url = file://s
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
  the active image in place and warns when a newer macOS exists; `--update`
  moves to it; an image's IPSW is deleted once it is prepared; builds use the
  image prepared last; `--image` and a directory source pick another), the
  build-link table row and the paragraph after it.
- `cmd/warden/image.go`: the `image prepare` help (`--driver vz` line,
  `--update`, Example with a local `.ipsw` path).
- `docs/book/src/concepts/overview.md`, VM drivers: the stamp line's vz fix,
  and one line for the vz base image choice (`--image`, else the cache's
  active image).
- `docs/book/src/guide/cli.md`: `warden image prepare` (the vz row of the
  source table: a local `.ipsw` path is read in place and kept, a URL is
  downloaded and its copy deleted once prepared; the examples) and `warden image list` (the example output,
  which also shows a stale `vz/images/macOS-15.4` path today).

## Defaults and overrides

Recorded at Publish from the approved Decisions, after `main` added this
section to the template (GLOBAL-09 to GLOBAL-11, #104); no decision changes.

- **Default: no source refreshes the active image in place** (Decision 2).
  The obvious meaning of "prepare again" is "re-prepare what I build with",
  which the guide already promised; it makes no IPSW lookup before the
  refresh. With no prepared image at all it restores the latest supported
  IPSW, as first-time setup always has.
- **Default: a build without `--image` uses the active image** (Decision 1),
  the one prepared last.
- **Overrides, each used exclusively (GLOBAL-10):** a source (an IPSW URL, a
  local `.ipsw`, or a prepared image directory) prepares exactly that and
  makes it active (Decisions 4, 7); `--update` restores, or refreshes, the
  latest supported IPSW and makes it active (Decision 3); `--update` with a
  source is refused, and on any driver but vz it is refused before anything
  runs. `--image` on a build uses that disk, whatever is active.
- **Fallback 1: the newest `.prepared` image when there is no valid active
  pointer** (Decision 1).
  - *Trigger:* `<cache>/active` is absent (every cache prepared before this
    change) or names a missing or invalid image directory.
  - *How it is seen:* `image list` marks the image builds will use as active,
    and `doctor --driver vz` reports that image's stamp; the next prepare
    writes the pointer.
  - *What it never does:* pick an image that is not a valid prepared
    directory; ties break by name, so the choice is deterministic.
- **Fallback 2: the out-of-date lookup is best effort** (Decision 3).
  - *Trigger:* after a successful no-source refresh, the lookup of the latest
    supported IPSW fails (offline, Apple unreachable).
  - *How it is seen:* no out-of-date warning is printed; the refresh itself
    has succeeded and reports so. Native case 6 covers it.
  - *What it never does:* fail the refresh, download anything, or change the
    active image.

## Cross-driver impact

- **vz:** everything above; `driver/vz/image.go`, `driver/vz/driver.go`
  (`resolveImage`, `imageStamp`, `LauncherStamps`) and the vz branches of
  `cmd/warden/image.go` (`prepareVZMacos`, `runImageList`);
  `driver/vz/environment.go` (`ipswInput` records `file://<path>` for a local
  file). `resolveImage` reaches `RestoreIPSW` too, so `build --image
  <local .ipsw>` also stops failing in curl; its restore still writes no
  pointer (Decision 8).
- **qemu, firecracker, hyperv, container:** unaffected. No shared package
  changes; `cmd/warden/image.go`'s other branches and `imageSource` are
  untouched; `--update` is accepted only on the vz path and refused on the others
  with a "not supported yet" error (ORCH-01). Hyper-V
  already has an explicit active base.

## Validation plan

- **Tier 0 (every OS, `make test`):** `classify` for each source form (URL,
  `file://` URL, absolute and relative local IPSW path, prepared directory and
  its `disk.img`, a path that is none of these, refused naming it);
  `--update` alone, with a source (refused), and on another driver (refused);
  the IPSW deletion after each prepare path, and a local IPSW source left in
  place;
  a local IPSW restore with a stubbed `restoreIPSW`: it receives the file's own
  path, no download runs (curl not invoked), no `ipsw-<key>.ipsw` is written,
  the image is `macos-<shortHash(abs path)>` whether the path was given
  relative or absolute, the recorded input is `file://<abs path>` with the
  file's digest, and the file still exists afterwards; a second prepare of the
  same path refreshes in place;
  `ImageDir` with a pointer, without one (newest `.prepared`, name tie-break),
  with a pointer to a missing or invalid dir; the prepare decision with a
  stubbed lookup (each row of the switch, including an IPSW whose dir sorts
  second, and that no lookup happens for an empty-source refresh before it
  succeeds); the out-of-date warning (`OutOfDate`): off when the active
  directory is `u`'s key, off when the active image's recorded IPSW file name
  equals `u`'s (a local-file image), on with the "about 25 GB" text when `u`
  has no prepared image, on with the "already prepared, no download" text when
  it has one, on naming `macos-<key> (IPSW not recorded)` when no provenance
  is recorded, versions parsed from Apple file names and the file name used
  otherwise, and a lookup error staying silent;
  `imageStamp`'s three fix texts; `image list` formatting from a fake cache,
  including a local-file image's IPSW name (GLOBAL-04).
- **Tier 2, native, blocking: macos-vz** (FLOW-09). Run on Jeff's
  Apple-silicon Mac, whose cache still holds `macos-90a1b1f94f64b00a`
  (27.0.1, unstamped), `macos-a139c218a2010753` (27.2, stamped) and both
  IPSWs, with no `active` file, so the pre-change cache is the starting point.
  Before case 2, stage case 7 for free: hard-link the cache's real 27.2 IPSW
  under Apple's name outside the cache (`ln ~/.cache/warden/images/ipsw-a139c218a2010753.ipsw
  ~/UniversalMac_27.2_26B123_Restore.ipsw`, same volume, no copy), so the
  file survives case 2's deletion of the cache name.
  1. A build with no `--image` uses `macos-a139…` (newest `.prepared`);
     `image list` marks it active and lists `macos-90a1…` as older with its
     IPSW name; `doctor --driver vz` reports `macos-a139…`'s stamp.
  2. `sudo warden image prepare --driver vz` with no source refreshes
     `macos-a139…` in place: no IPSW download or restore, "Refreshing boot
     agent", `active` written and owned by the user, `ipsw-a139….ipsw`
     deleted and `ipsw-90a1….ipsw` kept, no out-of-date warning (27.2 is
     current); then a build with no `--image` succeeds and its ledger
     verifies.
  3. `sudo warden image prepare --driver vz ~/.cache/warden/images/macos-90a1…`
     refreshes the old image in place (no restore), stamps it, makes it
     active and deletes `ipsw-90a1….ipsw`; a build with no `--image` boots
     27.0.1 and its ledger verifies. A no-source refresh now ends with the
     out-of-date warning naming 27.0.1, 27.2 and `--update` (the "already
     prepared, no download" form, since `macos-a139…` is prepared).
  4. With `macos-90a1…` active, `--image ~/.cache/warden/images/macos-a139…/disk.img`
     after removing that image's `link-channel` is refused naming `sudo warden
     image prepare --driver vz <macos-a139… dir>`; running it fixes the build
     and makes `macos-a139…` active.
  5. `sudo warden image prepare --driver vz --update` (27.2 already prepared)
     refreshes `macos-a139…` in place with no download and makes it active;
     `--update` with a source, and `--driver qemu --update`, are refused.
  6. With the Mac offline, `sudo warden image prepare --driver vz` still
     refreshes the active image and prints no out-of-date warning or error.
  7. `sudo warden image prepare --driver vz ~/UniversalMac_27.2_26B123_Restore.ipsw`
     (a plain path; refused by curl before this change) restores from the
     file in place: no "Downloading IPSW", no new `ipsw-*.ipsw` in the cache,
     the file still present afterwards, a new `macos-<key>` made active, and
     `image list` showing it with that file name; a build with no `--image`
     succeeds and its ledger verifies; back online, a no-source refresh prints no
     out-of-date warning (the file name matches the latest IPSW's), and
     re-running the step 7 command refreshes in place. This is one restore
     (about 15 minutes); afterwards make `macos-a139…` active again (`prepare
     <macos-a139… dir>`) and remove the new image directory and the hard link.
- **Tier-2 gaps:** none. The out-of-date warning's positive case is staged
  natively by case 3 (27.0.1 active, 27.2 the latest), the local-file restore
  by case 7.

## Implementation plan

Plan r2 (phase 2, `f2-plan-r2`, 2026-11-07, attended), written for the design
as approved at `e94a1be` (review r2), after reading `driver/vz/image.go`,
`driver.go`, `environment.go`, `vz_stub.go` and `cmd/warden/image.go` at
`5f33783`. The r1 plan round wrote no tasks (it returned the design, return
r2).

**Order.** At Jeff's direction the tasks are thin vertical slices (A to I),
each landing its failing tier-0 tests first and then the code that makes them
pass, so the decisions are exercised before the code settles. Tier 0 is pure
Go, table-driven, in `t.TempDir()` caches, and needs no root, network, curl,
`hdiutil` or VM: every host step is reached through the seams of task 6
(TEST-01, GLOBAL-07). Each task is marked **[native, no sudo]** when it can
also be checked early on the Apple-silicon Mac by an unprivileged run,
**[needs Jeff: sudo]** when its behavior shows only under a `sudo warden image
prepare`, or **[tier 0 only]**. Early native checks are informal previews; the
record is `f6-validate-macos-vz`, whose cases need the pre-change cache (no
`active` file), so no early check may run a prepare or write `active` (only
Jeff's f6 session does).

Tasks 2, 4, 7, 9, 11, 13, 15, 17, 19 and 20 rework code that already exists
(FLOW-06): `ImageCache.ImageDir`, `RestoreIPSW`, `ensureIPSWDownloaded`'s
caller, `ipswInput`, `imageStamp`/`checkLink`/`LauncherStamps`,
`prepareVZMacos`, `runImageList` and the `image prepare` help. Tasks 21 to 23
are the reference-doc and `overview.md` updates the change owes (UX-01,
FLOW-15). No rule changes (`rule_changes: []`).

### A. Active image: pointer, `ImageDir`, `Images` (native case 1)

1. **Tests, tier 0** (`driver/vz/image_test.go`): `ImageDir` returns the
   `active` directory when it is valid; with no pointer, the valid directory
   with the newest `.prepared` mtime, the name breaking an mtime tie; with a
   pointer to a missing directory and to an invalid one (no `.prepared`), the
   same fallback; `""` for an empty cache. `SetActive` writes
   `macos-<key>\n` by temp file and rename, and `ImageDir` reads it back.
   `Images()` lists every valid directory once, with `Prepared` time, `Active`
   set on exactly the `ImageDir` one, and the IPSW file name read from the
   layer cache's recorded `ipsw` input (empty when not recorded). Rules:
   GLOBAL-04, GLOBAL-07, TEST-01, TEST-02 (tier 0). [tier 0 only]
2. **Code** (`driver/vz/image.go`, rework): `ImageDir` per Decision 1;
   `SetActive(dir)` (atomic rename, then `giveBackToSudoUser` on the file);
   `Images() []Image` (`Dir`, `Prepared`, `Active`, plus the recorded IPSW
   name Decision 5 lists); one helper `recordedIPSW(dir)` reading
   `LayerCache().Source(<dir>/disk.img)` inputs, shared with tasks 14 and 15.
   Builds, `doctor` and the stamp list pick the new choice up through
   `ImageDir` unchanged. Rules: UX-07, GLOBAL-05, ORCH-01. [native, no sudo:
   with no `active` file, `warden build` with no `--image` boots
   `macos-a139…` (newest `.prepared`) and `warden doctor --driver vz`
   reports its stamp, as f6 case 1]
3. **Tests, tier 0** (`cmd/warden/image_vz_test.go`): `image list`'s vz lines
   from a fake `[]vz.Image`: the active image marked `active`, others `older`,
   each with its IPSW file name, none for an image with no recorded IPSW, a
   stable order (active first, then newest prepared). Rules: GLOBAL-04,
   TEST-02 (tier 0), UX-05. [tier 0 only]
4. **Code** (`cmd/warden/image.go`, rework of `runImageList`'s vz branch): a
   pure formatter over `cache.Images()`, printed under `darwin` as today
   (`vz      macos    <dir>  active|older  <IPSW name>`). Rules: UX-05,
   ORCH-01. [native, no sudo: `warden image list` marks `macos-a139…` active
   and lists `macos-90a1…` older with its IPSW name, as f6 case 1]

### B. Source classification and `Prepare` with injected seams (Decisions 2 and 4)

5. **Tests, tier 0**: `classify` for `""` (empty); an `https://` URL and a
   `file://` URL (url); an absolute and a relative existing regular file
   (ipsw-file, made absolute; the relative case under `t.Chdir`); a prepared
   cache directory and its `disk.img` (prepared-dir, the directory); a
   prepared directory outside the cache (prepared-dir, flagged outside); a
   missing path and a directory that is not a valid image, each refused with
   `no IPSW file or prepared image at <path>; give an IPSW URL, a local .ipsw
   file, or a prepared image directory` (a `driver.Fixable`). Rules: UX-04,
   GLOBAL-04, TEST-02 (tier 0). [tier 0 only]
6. **Code** (`driver/vz/image.go`): the pure `classify(source, cacheDir)`;
   the seams on `ImageCache` the design names (the latest-IPSW lookup and the
   restore step) as unexported function fields, nil meaning the real one, plus
   the host steps that would otherwise stop a tier-0 run (the curl download,
   `RequireRoot`, and `ReprepareImage` as the refresh step); `RestoreIPSW`
   split (rework) into acquire, the injected restore step (`restoreIPSW`,
   `PrepareHeadlessImage`, first boot), provenance, stamp and `.prepared`,
   with behavior for a URL source unchanged and still writing no pointer
   (Decision 8). Rules: GLOBAL-07, TEST-01, ORCH-01. [tier 0 only]
7. **Tests, tier 0**: `Prepare` through the seams, one row per switch arm of
   the design's pseudocode: empty with an active image refreshes it and calls
   no lookup; empty with none prepared looks up the latest and restores it; a
   URL whose `macos-<key>` is not prepared restores it; a URL whose directory
   is prepared and sorts second in the cache refreshes that directory (the
   `existing == filepath.Dir(path)` defect); a prepared-dir source (directory
   or `disk.img`) refreshes it; a prepared-dir outside the cache is refused
   with `customImageFix` and no seam runs; every success ends with that image
   `active`; a failed refresh or restore leaves `active` as it was. Rules:
   UX-07, UX-04, GLOBAL-05, GLOBAL-04, TEST-02 (tier 0). [tier 0 only]
8. **Code**: `Prepare(source, update, latest)` per the pseudocode (refresh
   prints `Refreshing boot agent on <dir>`, which f6 case 2 looks for), ending
   in `SetActive`. Rules: UX-07, GLOBAL-05, UX-04. [tier 0 only]
9. **Code** (`cmd/warden/image.go`, rework of `prepareVZMacos`): `Preflight`,
   `RequireRoot`, then only `cache.Prepare`; the `existing ==
   filepath.Dir(path)` test and the direct `RestoreIPSW`/`ReprepareImage`
   calls go. Rules: ORCH-01, UX-04. [needs Jeff: sudo, f6 cases 2 and 3]

### C. `--update` and its refusals (Decision 3)

10. **Tests, tier 0**: `Prepare` with `update` and no source looks up the
    latest and restores it, or refreshes its directory when prepared, and
    makes it active; `update` with a source is refused with `--update picks
    the newest IPSW itself; drop the source, or drop --update to prepare that
    source` before any seam runs; a failed lookup under `--update` is an error
    (only the warning's lookup is silent). In `cmd/warden`, the `--update`
    check refuses every driver but vz with `--update is not supported for
    --driver <d> yet` and passes vz. Rules: UX-04, UX-03, ORCH-01, GLOBAL-04,
    TEST-02 (tier 0). [tier 0 only]
11. **Code**: the `--update` arm in `Prepare`; `flagImgUpdate` (`--update`)
    on `image prepare`, checked before the driver switch so another driver is
    refused before anything runs, and passed to `prepareVZMacos`. Rules:
    UX-03, UX-05, ORCH-01, UX-04. [native, no sudo: `warden image prepare
    --driver qemu --update` is refused (f6 case 5's second refusal); the vz
    refusals sit after the root check: needs Jeff: sudo, f6 case 5]

### D. An image's cached IPSW is deleted once prepared (Decision 6)

12. **Tests, tier 0**: after a URL restore, `ipsw-<key>.ipsw` is gone and the
    recorded `ipsw` input (URL, digest) was written before it went; after a
    refresh of an image whose `ipsw-<key>.ipsw` is present, it is gone; other
    images' `ipsw-*.ipsw` and `ipsw-latest.ipsw` stay; a failed prepare
    deletes nothing; a local IPSW source file stays (with task 16). Rules:
    GLOBAL-05, UX-07, GLOBAL-04, TEST-02 (tier 0). [tier 0 only]
13. **Code**: in `Prepare` after `SetActive`, remove
    `<cache>/ipsw-<key of result>.ipsw` if present; nothing else. Rules:
    GLOBAL-05, UX-07. [needs Jeff: sudo, f6 cases 2 and 3]

### E. The out-of-date warning (Decision 3)

14. **Tests, tier 0**: `OutOfDate` off when the active directory is
    `macos-<shortHash(u)>`; off when the active image's recorded IPSW file
    name equals `u`'s (a local-file image keyed by its path); on with the
    exact "is available. To update (about 25 GB, 15 minutes): sudo warden
    image prepare --driver vz --update" text when `u` has no prepared image;
    on with the exact "is already prepared. To use it (no download): sudo
    warden image prepare --driver vz --update" text when it has one; on
    naming `macos-<key> (IPSW not recorded)` with no provenance; versions as
    `macOS 27.2 (26B123)` parsed from `UniversalMac_27.2_26B123_Restore.ipsw`
    and the file name otherwise. Through `Prepare`: an empty-source refresh
    looks up only after the refresh succeeded, prints the warning to the
    injected writer, and a lookup error prints nothing and returns success;
    no lookup or warning after `--update`, a URL, an IPSW file, a prepared-dir
    source or a first-time restore. Rules: UX-04, GLOBAL-05, GLOBAL-04,
    TEST-02 (tier 0). [tier 0 only]
15. **Code**: `OutOfDate(active, u)` and the file-name parse; `Prepare`'s
    best-effort lookup after an empty-source refresh, writing `warning: ...`
    to stderr. Rules: UX-04, GLOBAL-05, UX-07. [needs Jeff: sudo, f6 cases
    2, 3 and 6]

### F. A local IPSW file restored in place (Decision 7)

16. **Tests, tier 0**: an ipsw-file source not yet prepared: the stubbed
    restore step receives the file's own path; the download seam is not
    called; no `ipsw-<key>.ipsw` appears; the image is
    `macos-<shortHash(abs path)>` whether the path was given relative or
    absolute; the recorded input is `url: file://<abs path>` with the file's
    SHA-256 and size; the file still exists afterwards; a second prepare of
    the same path refreshes in place. A `file://` URL source still goes
    through the download seam with its key unchanged. `image list`'s
    formatter shows a local-file image's file name. Rules: UX-03, GLOBAL-05,
    GLOBAL-04, TEST-02 (tier 0). [tier 0 only]
17. **Code**: `RestoreIPSW`'s acquire step by kind (url: `ensureIPSWDownloaded`
    unchanged; ipsw-file: the file in place, no curl, no copy, never
    deleted); `ipswInput` (`driver/vz/environment.go`, rework) records
    `file://<abs path>` for a local file. `resolveImage` is not touched
    (Decision 8). Rules: UX-03, GLOBAL-05, ORCH-01. [needs Jeff: sudo, one
    restore, f6 case 7]

### G. Stamp refusal fix texts (Decision 5)

18. **Tests, tier 0** (`driver/vz/link_test.go`, extending `TestImageStamp`):
    the active cache image's fix is exactly `PrepareCommand`; the fallback
    active image (no pointer) counts as active; a cache image that is not
    active gets `sudo warden image prepare --driver vz <dir>` and "omit
    --image to build from the active image"; a disk outside the cache keeps
    `customImageFix`; `LauncherStamps` (doctor) reports the active image.
    Rules: UX-04, GLOBAL-04, TEST-02 (tier 0). [tier 0 only]
19. **Code** (`driver/vz/driver.go`, rework of `imageStamp`, `checkLink`,
    `LauncherStamps`): the three fixes by the image's place (active, cache not
    active, outside), the active one found through `ImageDir`. Rules: UX-04,
    ORCH-01. [native, no sudo: with no `active` file, `warden build --image
    ~/.cache/warden/images/macos-90a1…/disk.img` (unstamped, not active) is
    refused naming `sudo warden image prepare --driver vz <macos-90a1… dir>`;
    nothing in the cache changes. Running that fix: needs Jeff: sudo, f6
    case 4]

### H. Help and reference docs

20. **Code** (`cmd/warden/image.go`, rework of the `image prepare` help): the
    `--driver vz` line (no source refreshes the active image in place and
    warns when a newer macOS exists; `--update`; a prepared image directory; a
    local `.ipsw` read in place), the `--update` flag text, and an Example
    with a local `.ipsw` path. Rules: UX-01, UX-05, UX-03. [native, no sudo:
    `warden image prepare --help`]
21. **Docs** (`docs/book/src/guide/drivers.md`): VZ First-Time Setup and the
    build-link table row and the paragraph after it, per the design's Docs
    list. Rules: UX-01. [tier 0 only]
22. **Docs** (`docs/book/src/guide/cli.md`): `warden image prepare`'s vz
    source row and examples, and `warden image list`'s example output
    (replacing the stale `vz/images/macOS-15.4` path). Rules: UX-01, UX-03.
    [tier 0 only]
23. **Docs** (`docs/book/src/concepts/overview.md`, VM drivers): the stamp
    line's vz fix, and one line for the vz base image choice (`--image`, else
    the cache's active image). Rules: FLOW-15, UX-01. [tier 0 only]

### I. Gate

24. **Gate**: `make test`, and `make lint` for darwin and for one non-darwin
    GOOS (driver/vz builds everywhere through `vz_stub.go`, which this change
    leaves unchanged); push the branch at each green slice. Rules: TEST-01,
    GLOBAL-07, FLOW-17. [native, no sudo]
25. **Native validation, tier 2, blocking** (`f6-validate-macos-vz`, phase 6,
    Jeff, sudo): cases 1 to 7 of the Validation plan as the step's note
    states them; this plan assigns them unchanged (case 1 to slice A, 2 and 3
    to B, D and E, 4 to G, 5 to C, 6 to E, 7 to F). Rules: FLOW-09, TEST-02
    (tier 2). [needs Jeff: sudo]

**Plan notes.** The design is workable as written; no finding returns it.
One statement for Consolidation, which needs no task: Cross-driver impact
says `build --image <local .ipsw>` "also stops failing in curl", but
`resolveImage` returns any existing path as the disk before it reaches
`RestoreIPSW`, so a local `.ipsw` there was never handed to curl, and
Decision 8 leaves `resolveImage` untouched; the plan follows Decision 8.

**Budget** (FLOW-13). 25 tasks in one `f3-implement` step. The estimate (2
runs, 9 steps) is spent with this step (2 runs, 9 steps). The rest is about 9
steps (implement, review, guide, merge-main, consolidate, open-pr, the native
step, close, one re-run) over about 3 runs: raise the estimate to about 5 runs
and 18 steps before the runaway check escalates.

## Design review

- **r1, 2026-11-06, Jeff: approved** with changes folded in, approved at
  f764e9f (read at 2ec271e). Decisions confirmed: the active pointer with the
  newest-`.prepared` fallback (a3); a prepared cache image as the source is
  refreshed and made active (a4). Folded in at f764e9f: a no-source refresh
  stays in place and ends with an out-of-date warning naming the update
  command (a1); a vz `--update` flag replaces the proposed `latest` source,
  refused with a source and on other drivers, with cross-driver `--update`
  queued as its own undesigned item (a2); each prepare deletes its image's
  cached IPSW while image directories stay (a5, replacing the
  `vz-image-retention` proposal). Validation cases 2, 3 and 5 updated to match.
- **Return r2, 2026-11-07 (Plan, FLOW-06; not a review).** Findings: (1) the
  out-of-date warning's condition (only when the latest IPSW had no prepared
  image) contradicted native case 3, which expects it with `macos-a139…`
  (27.2) prepared; (2) a plain local IPSW path cannot be restored (curl:
  "URL rejected: No host part"), although `classify`, Decision 6, the help and
  `cli.md` assume it can. Jeff decided both in the attended sitting. Changes
  since approved at f764e9f: Decision 3 warns whenever the active image is not
  from the latest supported IPSW, with a "no download" text when that image is
  already prepared, and defines the active image's IPSW identity (key, else
  the recorded IPSW file name); new Decision 7 restores a local IPSW file in
  place (no curl, no copy, never deleted, keyed by its absolute path,
  provenance `file://<path>`), with problem 3 and a refusal for a path that is
  no source (Decision 4); the out-of-scope note becomes Decision 8; the
  pseudocode, cache layout, tier-0 tests and native case 3 wording follow, and
  native case 7 (staged by a hard link before case 2) covers the local file.
  The f6-validate-macos-vz note was brought in line. Re-review pending
  (FLOW-19, `design_review: required`).

- **r2, 2026-11-07, Jeff: approved** as read, approved at e94a1be (re-review
  after the Plan return r2; previous approval f764e9f). Changes since r1, all
  from Jeff's answers at that return: the out-of-date warning fires whenever
  the active image is not the latest IPSW's, with an "already prepared, no
  download" form (Decision 3, the IPSW identity by key or recorded file
  name); a plain local IPSW path is restored from in place (Decision 7,
  fixing the curl "No host part" failure); native case 3 reworded and case 7
  added. a1 to a5 unchanged.

## Consolidation

The branch was cut at `main` `49e5354`. At f6-consolidate `main` had not
moved. Before the PR, `main` `97a13f7` was merged in (#104 to #107): process
rules, `.claude/` and `tools/workflow/` only, no code this design touches.
#104 added GLOBAL-09 to GLOBAL-11 and the template's **Defaults and
overrides** section; it is filled above from Decisions 1 to 3, naming the two
fallbacks (no valid active pointer, a failed out-of-date lookup). No decision
changes.

- **Seams reconciled:** the interface table under Image cache now matches the
  code. `Prepare(source, update)` takes no lookup argument; the host steps
  (`latest`, `download`, `restoreDisk`, `refresh`, the root check) are seams
  on `ImageCache`, nil meaning the real one, and `Image` carries the recorded
  IPSW file name that `image list` prints. Wording only; the decisions are
  unchanged.
- **Review SHOULD (UX-04), fixed here:** the `--update` refusal on a driver
  other than vz had an empty fix; it now reads "drop --update (only --driver
  vz supports it so far)", the pseudocode says so, and `TestCheckImageUpdate`
  asserts a non-empty fix.
- **Review hint dismissed (BOOT-07):** the change refactors how the host
  prepares and restores an image (`prepare.go`, the seams in `image.go`); the
  in-image bootstrap code (setup assistant skip, warden user, auto-login, the
  agents baked in) is unchanged, so BOOT-07 is not engaged.
- **Left as is (Decision 8):** the plan-r2 note on `resolveImage`, which
  restores an IPSW for `build --image <url>`: Decision 8 keeps it out of
  scope, and the code leaves it untouched.
- **Follow-up filed by Review:** vz-ipsw-latest-reuse (follows
  vz-image-prepare-refresh). `ensureIPSWDownloaded` prefers an existing
  `ipsw-latest.ipsw` for any URL, so an explicit IPSW URL can restore a
  different release. It is on `main` already and not this branch's work.
- **Terminology:** "active image", "refresh in place", "restore" and
  "out-of-date warning" are used alike in the design, `cli.md`,
  `drivers.md`, `installation.md`, `overview.md` and the help.
- **Sweep:** `make deadcode` reports the same set as `main` (the darwin-only
  callers of the vz bootstrap helpers, unreachable in a linux analysis); no
  new entry. No duplicated helper: `ipswFileName`, `ipswLabel`, `classify`
  and the seams each have one home in `driver/vz`. `overview.md` already
  carries the build's choice of the vz base image (the active image, else the
  newest prepared) and the stamp refusal's fix per image; `image prepare` has
  no pseudocode there, so nothing else changes.

## Rollback notes

## Archive summary

Designed 2026-11-07 (one return to Design, r2), approved at design review r1
(`f764e9f`) and r2 (`e94a1be`), reviewed by tier 2 at `967c35237998` (0 MUST,
0 MUST-review, 1 SHOULD, fixed at Consolidation), validated on macos-vz
(cases 1 to 7, 2026-11-11), published 2026-11-11.

- **Built.** The vz image cache records an active image, `<cache>/active`,
  written by every successful `image prepare --driver vz` and handed back to
  the sudo user. Builds without `--image`, `doctor` and the stamp list use it,
  falling back to the newest `.prepared` image (name tie-break). With no
  source, prepare refreshes the active image in place (boot agent, provenance,
  stamp) with no IPSW lookup, and then warns when the active image is not the
  latest supported IPSW's, in a "download" or an "already prepared, no
  download" form naming `--update`. `--update` (vz only) moves to the latest
  IPSW. A prepared image directory, or its `disk.img`, is a source that is
  refreshed in place and made active. A plain local `.ipsw` path is restored
  from where it is (no curl, no copy, never deleted; provenance
  `file://<path>`). Each prepare deletes its own cached `ipsw-<key>.ipsw`;
  image directories stay. `image list` prints every vz image, the active one
  marked, with its IPSW file name. The stamp refusal names the right fix per
  image.
- **Taken.** An active pointer over newest-marker or refusing (a3); refresh
  in place by default, the update only on request (a1); a flag, `--update`,
  over a `latest` source keyword, `--force` or a Hyper-V-style `auto` key
  (a2); the warning whenever the active image is not the latest's, even when
  the latest is prepared (return r2); a local IPSW restored in place
  (return r2); delete the IPSW, keep the images (a5).
- **Rejected.** Picking up a new macOS at refresh (a different build OS,
  changed silently); deleting older images (they are the user's rollback);
  refusing a plain local path; copying a local IPSW into the cache.
- **For a future design touching these concepts.** The active image's IPSW
  identity is its key `macos-<shortHash(source)>` or, failing that, the file
  name of its recorded IPSW URL; the digest is never compared. `active` lives
  in `~/.cache/warden/images`, shared with the qemu images; a more specific
  name would avoid any later collision. Cross-driver `--update` is queued as
  `cross-driver-image-update`; `ensureIPSWDownloaded` preferring
  `ipsw-latest.ipsw` for any URL is `vz-ipsw-latest-reuse`; `resolveImage`'s
  IPSW restore for `build --image <url>` writes no pointer (Decision 8). Apple
  released 27.2.1 during validation: the warning and `--update` behaved as
  designed against a newer IPSW than the plan assumed.
