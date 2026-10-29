---
feature: hyperv-windows-prepare
status: archived
round: parallel
drivers: [hyperv, qemu]
seams: [image-manifest, windows-guest-seed]
surfaces: [cli, config, output]
security_boundary: none
alters_relationships: true
native_validation:
  - {host: windows-hyperv, blocking: true, reason: "changes the Hyper-V Windows guest lifecycle end to end"}
  - {host: macos-qemu, blocking: true, reason: "qemu Windows path moves to the shared guest package and prepare-time tuning"}
tier2_gaps: []
rules: [FLOW-01, FLOW-02, FLOW-06, FLOW-07, ORCH-01, UX-01, UX-04, GLOBAL-06, GLOBAL-08, TEST-02, TEST-03]
concepts: [image-prepare, generalized-base, sysprep, windows-guest, eval-licensing, differencing-disk, image-refresh, guest-tuning]
related: [vm-driver-release-plan.md, hyperv-driver-plan.md, windows-driver-plan.md]
---

# Hyper-V Windows guests: prepare once, refresh on the grace clock

## Problem

Every Hyper-V Windows build boots the stock eval image and runs specialize plus
OOBE, then runs the build as the auto-logged-on Administrator. The qemu Windows
path instead installs once (`image prepare`) and runs each build from a
differencing child via a SYSTEM startup task. The two paths keep separate copies
of `warden-run.ps1` and the unattend generator, and differ in seed media, build
account and lifecycle (Phase 5a in `vm-driver-release-plan.md`).

Measured on the Windows desktop (2026-10-20, Gen2 Server 2025 Datacenter eval
VHDX):

- A trivial build takes 81 s end to end; numpy-source-win takes 488 s. OOBE is
  a bounded cost, not the dominant one; in-guest background work (Defender,
  Windows Update, activation) also lands in every build.
- Right after OOBE the guest is unactivated, in the **10-day initial grace
  period**, with an absolute end date (wall clock, not uptime) and one rearm. A
  long build activates online through the relay and discards it with the VM.

## Prior art

- Phase 5a hypothesis and checklist (`vm-driver-release-plan.md`): this design
  adopts the prepare-once chain and answers checklist items 1, 3 and 5.
- qemu `image prepare --os windows` (`driver/qemu/winacq.go`,
  `autounattend.go`): the startup-task contract, the 1024-char
  FirstLogonCommands limit and its length test are reused as-is.
- Hyper-V seed (`driver/hyperv/winseed*.go`): COM1 mirroring and the
  `-EncodedCommand` drive scan are kept.
- Ledger environment types (separate design, in progress): the prepared layer is
  recorded as a `vm-disk-chain/v1` layer with its preparation inputs.

## Decisions

1. **Prepare once into a differencing chain.** Chain: pinned eval image
   (never modified) -> prepared layer -> per-build child. *Rejected:* per-build
   OOBE with shared generators (keeps the 81 s floor and per-build background
   work); flattening with Convert-VHD (copies the 11 GB parent per refresh);
   saved-state restore (shared RNG state, hard to clone, captures runtime memory
   that is hard to account for).
2. **Prepare is network-free and is refreshed before the 10-day grace runs out.**
   The prepared base is never activated. *Rejected:* activating during prepare
   through a relay (180 days per prepare, but makes prepare depend on reaching
   Microsoft through whatever proxy or firewall sits in front of the host; a
   network-free prepare works end to end on filtered networks). The cost is a
   refresh every 7 days (3 days short of the grace end, for long builds);
   prepare is short, so this is accepted churn.
3. **Builds run as SYSTEM from the `warden-run` startup task**, as on qemu. No
   auto-logon, no user profile creation, no shell on build boots.
4. **One Windows guest package shared by qemu and hyperv**
   (`driver/winguest`): `warden-run.ps1`, the startup-task registration command,
   the oobeSystem unattend fragment, the prepare-time tuning script, and the
   grace-clock report. Driver-specific parts stay in the drivers: qemu's
   windowsPE/install passes and FAT seed, Hyper-V's ISO seed and Gen1/Gen2
   handling. Parameters, not forks, cover the differences (gateway and CIDR,
   COM1 mirroring on or off).
5. **Tune once at prepare.** Disable Windows Update (wuauserv, UsoSvc),
   DiagTrack, SysMain, Server Manager at logon, maintenance and defrag tasks;
   remove Windows Defender; run the .NET ngen queue to completion; high
   performance power plan, hibernation off; automatic activation off (so builds
   no longer contact the activation service); and the remaining background
   network calls every build would otherwise make through the relay: the
   connectivity probe, root/disallowed certificate list updates, and
   ClipSVC/LicenseManager license sync. Revocation checks a build itself makes
   (CRL/AIA) are left alone. The guest is the untrusted side
   and the VM plus relay are the boundary, so in-guest AV adds no isolation.
6a. **Base images: latest from Microsoft, provenance instead of pinning**
   (decided 2026-10-27). The base comes from `--source`, else `[vm]
   windows_image` (URL or local path), else the registry default: for Server
   2025, Microsoft's Evaluation Center fwlink
   (`go.microsoft.com/fwlink/?linkid=2293215`), which Microsoft repoints at its
   current media. A HEAD request resolves it; the file is downloaded under its
   own name beside any previous base, and the resolved URL, ETag,
   Last-Modified and computed sha256 go into the manifest and the ledger. No
   hash is pinned in code and `--accept-unpinned` is gone; `--sha256` remains
   an optional check. Each refresh re-checks a URL base and switches to new
   media by default (`[vm] windows_image_update = "auto"`; `"reuse"` keeps
   the current one); a failed check only warns, so refresh still works offline.
   A base supplied as a local file is never replaced automatically. A replaced
   base is deleted once no prepared layer or VM uses it. *Rejected:* pinning a
   hash in code (Microsoft publishes none, and it would freeze the media);
   scanning the user's Downloads folder for a matching file (explicit sources
   only).
6. **At most one prepare per driver cache at a time**, enforced by an OS file
   lock, with an atomic switch of the manifest's `active` pointer. Whoever
   takes the lock first reaps what an interrupted prepare left (Hyper-V): any
   `warden-prepare-*` VM, unrecorded layer files and `work-*` seed dirs in
   `prepared/`, keeping anything a VM still uses. Holding the lock means no
   live prepare owns them; a `--keep-vms` prepare VM is reaped by the next
   prepare.
7. **Two image stages, the same for both drivers.** A *generalized base* is an
   installed, sysprepped disk with no running grace clock: either Microsoft's
   eval VHD/VHDX, or the output of warden's ISO *install stage*. The *prepared
   layer* (OOBE + tuning, Decisions 1-5) is always built from a generalized
   base and is what the 7-day refresh redoes. The ISO install stage runs
   `sysprep /generalize /oobe /shutdown` as its last first-logon step, with
   `PersistAllDeviceInstalls=true` (virtio drivers survive generalize) and
   without `SkipRearm` (so the install-time grace clock does not carry into the
   base). It is redone only when its inputs change, never for the grace clock.
   *Rejected:* refreshing qemu by reinstalling from ISO (a full install per
   week); `SkipRearm` (the base would carry its install date's grace clock).

## Questions

### Blocking

None.

### Assumed

- **Refresh thresholds.** Refresh by age, not by remaining grace: background
  refresh once the active base is 7 days old (`[vm] image_refresh_after`, default
  `7d`), leaving a 3-day margin of grace so very long builds (monorepos) never
  run into the post-grace shutdown. Separately, a build refuses to start when
  less than `[vm] image_min_grace` (default `1d`) remains and says to run
  `warden image prepare` (UX-04). Confirm the defaults.
- **Grace end measured once, at prepare.** The clock is wall time, so prepare
  records the guest's reported grace end (`slmgr /xpr` /
  `GracePeriodRemaining`) in the manifest and builds compute remaining time from
  that, with no per-build guest report. Confirm.
- **No rearm.** `slmgr /rearm` is not used (one rearm available, and it makes the
  base depend on its own history). Confirm.
- **A warden-generalized base behaves like Microsoft's.** Measured on the
  Microsoft image: specialize starts a fresh 10-day grace and leaves the rearm
  count at 1. Proceeding on the assumption that an ISO-installed, generalized
  base does the same; validated first on qemu (see Validation plan). Confirm.
- **Grace refresh applies to eval editions only.** A retail or volume-licence
  base uses the same stages but activates normally; prepare records its license
  status and skips `grace_end` when the channel is not `TIMEBASED_EVAL`.
  Confirm.
- **Defender removal over exclusions.** Removal needs a reboot inside prepare;
  exclusions would leave real-time scanning on for system paths. Proceeding with
  removal. Confirm.

## Design

### `warden image prepare --driver hyperv --os windows`

Flags: `--source`, `--sha256`, `--force`, `--name`, `--keep-vms` (base-image
sourcing per Decision 6a). `--source` may also be a local VHD/VHDX, registered
in place (no copy; the disk-hash cache avoids re-hashing it). With no source,
digest or `--force`, prepare is the refresh: it re-checks a URL base for new
media (unless `windows_image_update = "reuse"`) and re-prepares, so there is no
separate `--refresh` flag. Steps:

1. Take the prepare lock (`driver.TryHostLock("hyperv-prepare")`, an OS file
   lock, non-blocking). Held: exit 0 with "a prepare is already running".
2. Resolve the active generalized base from the manifest.
3. Create `prepared/<name>-<timestamp>.vhdx` (or `.vhd` for Gen1) as a
   differencing child of the base.
4. Build a prepare seed ISO labelled `WPREP`: the OOBE answer file
   (`winguest.OOBEUnattend`, `LogonCount` 1) under both implicit-search names,
   and `warden-prepare.ps1` (`winguest.PrepareScript`).
5. Create the VM with **no network adapter** (an empty `SwitchName` now removes
   the adapter New-VM always adds) and no COM1 (VM lifecycle privilege only;
   no Mount-VHD).
6. Boot. OOBE runs; the one FirstLogonCommand runs `warden-prepare.ps1`, which
   applies the tuning. When Defender removal leaves a reboot pending it
   registers a one-shot SYSTEM startup task for its `finish` stage and
   restarts the guest itself; the host only waits for the VM to reach Off.
   `finish` runs the ngen queue, registers `warden-run`, reports the license,
   reports `done`, clears its reported items and powers off.
7. **Progress channel: Hyper-V key-value pair exchange, not COM1.** The guest
   writes values under `HKLM\SOFTWARE\Microsoft\Virtual Machine\Guest`; the
   host reads them from `Msvm_KvpExchangeComponent.GuestExchangeItems` over
   WMI (no elevation). COM1 was rejected: connecting to a Gen2 VM's COM1 pipe
   during UEFI stops the guest booting, and prepare reboots. Items:
   `warden-prepare-state` (`running:<step>`, `done`, `error:<msg>`),
   `warden-grace-minutes`, `warden-license-status`, `warden-license-channel`,
   `warden-reported-at`. The grace end is the reading time plus the minutes,
   recorded only for the `TIMEBASED_EVAL` channel.
8. Timeout (45 min default) or power-off before `done`: a fixable error naming
   the last guest state and the guest log; the partial layer is deleted and
   the active layer is unchanged.
9. Hash the prepared layer, record it in the layer cache as a `prepared`
   child of the base with the seed files as inputs, mark it read-only, append
   it to the manifest and make it active (temp file + rename).

### ISO install stage (qemu now; any driver later)

`warden image prepare --driver qemu --os windows` keeps its ISO + virtio-win
inputs, but its result is a generalized base rather than a build-ready image:
the existing unattended install, then `sysprep /generalize /oobe /shutdown`.
The base is hashed, pinned and recorded with the sha256 of the ISO, the
virtio-win ISO and the install answer file. The prepared layer is then built
from it exactly as on Hyper-V. `--source` with an already installed,
generalized VHD/VHDX (converted with `qemu-img` for qemu) skips the install
stage on either driver.

As implemented (step 2a):

- The install's single FirstLogonCommand is `winguest.GeneralizeCommand`: it
  deletes the answer file Setup cached in `%WINDIR%\Panther` (otherwise the
  next OOBE would reuse it, autologon and generalize again instead of reading
  the prepare seed's), enables the built-in Administrator that prepare's OOBE
  logs on as, disables the install account, and runs sysprep. AutoLogon is
  limited to one logon so no password is left in Winlogon.
- The install VM has no network (`-nic none`): Setup bypasses OOBE's network
  check and NetKVM comes from the virtio-win ISO, and without a network the
  Store cannot update an app for the install user, the usual reason sysprep
  refuses to generalize a Windows 11 image.
- A `windows-<arch>.qcow2.generalized` marker tells a generalized base from one
  installed before this change (which carries `warden-run` itself and still
  boots as is). A reinstall deletes the prepared layers, whose backing file it
  replaces.
- The prepare stage boots a qcow2 overlay with its own copy of the UEFI vars,
  no network, and the WPREP seed as a writable FAT disk. qemu has no KVP, so
  `warden-prepare.ps1` also writes its reported items as a sequenced status
  block to that volume, and the host scans the raw image for the newest block
  after qemu exits (`winguest.ParseSeedStatus`).
- qemu keeps no host-wide VM registry, so retirement keeps the active layer
  and the one it replaced and deletes older ones without an in-use check.

Assumed, to validate on the macOS host (blocking): the built-in Administrator
can autologon in prepare's OOBE on a Windows 11 client image once enabled
before generalize (Server enables it by default); and a generalized ISO
install gets a fresh 10-day grace at prepare, like Microsoft's image.

### Manifest

`images/index.json` records generalized bases (path, sha256, source URL or
install inputs) and, per prepared layer: `path`, `sha256`, `size`,
`mtime`, `parent` (generalized base name + sha256), `generation`, `prepared_at`,
`grace_end`, `warden_version`, and `inputs` (sha256 of the unattend,
`warden-prepare.ps1`, and the tuning list). These map directly onto a
`vm-disk-chain/v1` prepared layer once the ledger environment types land.

### Per build

- Resolve the active prepared layer and its base (both files must exist; a
  moved cache dir fails with "re-run `warden image prepare --driver hyperv`",
  checklist item 4). A pinned base that was never prepared is a fixable error:
  builds boot only prepared images. `--image` still names a single disk, which
  must already carry the `warden-run` task.
- Freshness: under `image_min_grace` left, refuse (UX-04); older than
  `image_refresh_after`, start a detached `warden image prepare --driver
  hyperv` (logging to `<cache>/prepare.log`) and continue on the current base.
  The lock makes repeated starts harmless.
- Differencing child of the prepared layer plus a `WARDEN` seed carrying only
  `warden-run.ps1` and `warden-io.exe`. The startup task finds the seed volume,
  runs `warden-run.ps1` as SYSTEM, and powers off, as on qemu.

### Retiring old prepared bases

At the end of each prepare, layers older than the active one and the one it
replaced are deleted, unless a VM still depends on them (every attached disk
walked up its `Get-VHD ParentPath` chain). Keeping the previous layer covers a
build that resolved its chain just before the switch. Failures are logged,
never fatal.

### `image list` and `doctor`

Show the active base, its parent, `grace_end` and days remaining, whether a
refresh is running (lock held), and older bases awaiting retirement. `doctor`
warns once the base is past `image_refresh_after` with no refresh running, and errors
under `image_min_grace`.

## Cross-driver impact

- **hyperv:** new prepare path, per-build lifecycle switches to startup task +
  prepared chain; the per-build OOBE, the FirstLogonCommand-per-build path and
  the hyperv copies of `warden-run.ps1` / unattend are removed.
- **qemu:** `warden-run.ps1`, task registration and tuning move to `winguest`.
  `image prepare` splits into the ISO install stage (ending in generalize) and
  the shared prepared-layer stage with tuning and `grace_end`. FAT seed
  unchanged.
- **container, vz:** unaffected (no Windows guests).
- **interface:** none. Prepare stays a driver-level image operation behind the
  existing `image prepare` CLI.

## Security note

`security_boundary: none`. Removing Defender and Windows Update changes the
guest's internal posture only; the guest is already the untrusted side, and the
host/guest and guest/relay boundaries are unchanged. Prepare runs with no
network adapter, so the only inputs to the prepared layer are the base image
and the seed, whose hashes are recorded.

Base-image trust (Decision 6a) changes the original Hyper-V plan's
secure-by-default rule (`hyperv-driver-plan.md`: refuse an image with no
pinned hash unless `--accept-unpinned`). The default download is now trusted
on HTTPS to a Microsoft host plus recorded provenance, with no prior hash:
Microsoft publishes none for its evaluation media, and pinning one in code
would freeze the media. What a build booted is still exactly identified
(resolved URL, ETag, sha256 in its signed ledger) and can be compared across
builds and hosts after the fact; `--sha256` restores a prior check for anyone
who has an independent digest. An explicit `--source` / `[vm] windows_image`
URL is the operator's choice and has no host restriction. The prepared layer is a cached
trust input: it is hashed at prepare, checked cheaply per build, fully re-hashed
by `warden image verify` / `doctor`, and marked read-only.

## Validation plan

- **Tier 0/1:** unit tests for the shared generators (FirstLogonCommand length
  limit, COM1 on/off rendering, gateway parameters), manifest round-trip and
  atomic swap, lock contention (second prepare exits cleanly), grace thresholds,
  COM1 line parsing, retirement selection with a fake provisioner.
- **Tier 2, windows-hyperv (blocking):** prepare from the Gen2 eval VHDX;
  conda-forge-win and numpy-source-win from the prepared base as SYSTEM
  (checklist item 1); per-build wall time against the 81 s / 488 s baselines
  (item 5); a concurrent second prepare; a refresh while a build runs, then
  retirement of the old base; a moved cache dir is detected; grace end recorded
  matches `slmgr /xpr`; a base older than `image_refresh_after` triggers exactly one
  background refresh.
- **Tier 2, macos-qemu (blocking):** first, an ISO-installed generalized base,
  then prepared: "Initial grace period" counted from the prepare, not the
  install, and rearm count unchanged. Then the qemu Windows build passes with the
  shared package and tuning, and a refresh from the generalized base does not
  reinstall.
- **Out of scope:** Gen1 `.vhd` (IDE DVD seed discovery, a two-level `.vhd`
  chain). Microsoft's current official download is a Gen2 VHDX; the Gen1 code
  paths stay but are unvalidated, and validating them is a feature request if
  someone needs a `.vhd` (decided 2026-10-26). Upstream CA certificates are
  a relay-side check, not a Windows one.

## Implementation plan

1. `driver/winguest`: move and parameterize `warden-run.ps1`, task
   registration, oobeSystem fragment; add `warden-prepare.ps1` (tuning, grace
   report, reboot-until-done). Tests, including the 1024-char guard. (ORCH-01,
   GLOBAL-08)
2. Rework qemu to use `winguest` (explicit rework task, FLOW-06); no behaviour
   change beyond tuning and the grace record.
2a. Split qemu `image prepare` into the ISO install stage (generalize, pin the
   base) and the shared prepared-layer stage (explicit rework task, FLOW-06).
3. Hyper-V prepare: lock, differencing layer, no-NIC VM, COM1 parsing,
   reboot-until-done loop, hash, manifest, atomic swap. (UX-04)
4. Hyper-V per-build lifecycle on the prepared chain via the startup task;
   remove per-build OOBE and the hyperv generator copies.
5. Grace checks, detached refresh, retirement of old bases.
6. `image list` / `doctor` output; reference docs for the new flags and output
   in the same change (UX-01).
7. Native validation runs above; record results in this doc.

Status: steps 1-6 implemented on `feat/hyperv-phase5a`; step 2a (qemu ISO
install ending in generalize) and the macOS qemu validation are not done.

## Validation results

**windows-hyperv, 2026-10-20**, Gen2 Server 2025 Datacenter eval VHDX
registered in place (`--source <path> --accept-unpinned`), service and relay
boot disk from this branch.

| Check | Result |
|---|---|
| Prepare (OOBE, tuning, Defender reboot, ngen, task) | Pass, 195-196 s on three runs |
| No network during prepare | Pass: VM created with no adapter |
| Progress over KVP through the reboot | Pass: `running:<step>` ... `done` |
| Grace end recorded | Pass: prepare + 10 days, matches the guest's `slmgr /xpr` |
| Builds as SYSTEM from the startup task (item 1) | Pass: `nt authority\system`; MSVC, conda and pip all work |
| Grace clock is wall time from prepare (item 3) | Pass: 14395 of 14400 min left 5 min after prepare |
| Per-build time (item 5) | trivial build 80 s -> 36 s; conda-forge-win 71 s; numpy-source-win 412 s and 515 s (dominated by the ~2 GB VS Build Tools download, so not a reliable comparison with the earlier 488 s) |
| Ledger chain | Pass: eval image (`local`) then the prepared layer (`prepared`, 3 hashed inputs); signatures valid |
| Second concurrent prepare | Pass: exits 0 at once, "already running" |
| Retirement | Pass: the third prepare deleted the first layer and kept the second |
| Tuning applied | Pass: services disabled, policy keys set, Defender removed, High performance, no leftover KVP items |
| Activation traffic in builds | Gone after disabling the SoftwareProtectionPlatform tasks |
| Other background traffic in builds (2026-10-26) | Gone: the connectivity probe (`NoActiveProbe`), root and disallowed certificate list updates (`DisableRootAutoUpdate`, `EnableDisallowedCertAutoUpdate=0`), and `licensing.mp` / `slscr.update` (ClipSVC and LicenseManager disabled). conda-forge-win 58 s with no background calls; numpy-source-win 482 s, VS Build Tools still verifies its signatures; what remains is the installer's own CRL and AIA fetches for those checks |

**windows-hyperv, 2026-10-26** (after the qemu step 2a refactor):

| Check | Result |
|---|---|
| Prepare with the shared winguest code | Pass, 199 s; the seed-status write to Hyper-V's read-only ISO is skipped silently |
| Pin a local base with `--sha256` | Pass: manifest and ledger record the base as `local`, `pinned` |
| Trivial build | 31-34 s; the first build after a prepare took 85 s (no host-side hashing, so most likely a cold host cache for the new layer) |
| Refresh at `image_refresh_after` (layer back-dated 8 days) | Pass: the build prints the refresh notice and succeeds on the current layer; a detached prepare runs, a second build meanwhile starts no other, and the new layer becomes active |
| Refusal under `image_min_grace` (grace end 2 h away) | Pass: the build fails in 1 s with the prepare fix; `doctor` exits 1, "Not ready" |
| Retirement with a layer in use | Pass: a `--keep-vms` build VM on a layer kept it through a prepare that would otherwise delete it; after removing the VM the next prepare deleted it |
| Moved or deleted layer | Pass: the build fails in 1 s, "chain is incomplete: <path> is missing" |
| Guest powers off without reporting (agent killed before its first heartbeat) | **Bug found and fixed**: the host waited until `--timeout` (15 min; forever without one). The Hyper-V host now watches the build VM and fails 5 s after it is off with no completion (40 s end to end) |
| Heartbeat silence with the guest still up | Pass: "build guest unresponsive (no heartbeat for 30s)" (73 s end to end); now a fixable error pointing at build-output.log instead of a bug-report link |

Not run: macos-qemu. Gen1 `.vhd` is out of scope (see Validation plan).

## Consolidation (merge round)

_To be filled in during the merge round._

## Rollback notes

- **COM1 for prepare progress (design step 7) replaced by KVP.** Evidence:
  `tools/relay-vm/hyperv/read-serial-pipe.ps1` documents that a client on a
  Gen2 COM1 pipe during UEFI prevents boot, and prepare reboots. A spike
  confirmed guest-written items under `...\Virtual Machine\Guest` reach the
  host without elevation (`...\Auto` holds only intrinsic items).
- **`Report` must not `New-Item -Force` the existing KVP key**: it recreated
  the key and dropped every earlier value, so the first prepare recorded no
  grace end. Fixed and guarded by a unit test.
- **`Activation\Manual = 1` alone did not stop activation**; the
  SoftwareProtectionPlatform restart tasks also had to be disabled.

## Archive summary

Merged 2026-10-28 in buildwarden/buildwarden#60 (with `unified-bootstrap`).

- **Built.** Windows guests on Hyper-V and qemu boot a per-build child of a
  prepared layer over a never-modified generalized base. Prepare runs once, with
  no network, tunes the guest (Update, telemetry, activation, Defender, the
  background calls), registers the `warden-run` startup task, and reports
  progress over KVP. Builds run as SYSTEM. Builds refresh the layer in the
  background after 7 days and refuse with less than a day of evaluation grace.
  The base defaults to Microsoft's current eval download; `--source` takes a URL
  or a file registered in place, which is never deleted. One prepare at a time;
  an interrupted prepare's VM and files are reaped by the next. One Windows guest
  package (`driver/winguest`) for both drivers.
- **Rejected.** Per-build OOBE; activating during prepare; COM1 for progress
  (it blocks Gen2 boot during UEFI, so KVP instead); reinstalling qemu from ISO
  on refresh; scanning Downloads for media.
- **For future designs.** Gen1 `.vhd` is out of scope. macos-qemu validation of
  the shared guest package was not run. The prepared cache must be visible to
  Hyper-V's service (see `unified-bootstrap`'s MSIX note). A download's file
  name comes from the server's final URL and is checked to be one plain name.

