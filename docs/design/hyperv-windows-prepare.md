---
feature: hyperv-windows-prepare
status: draft
round: parallel
drivers: [hyperv, qemu]
seams: [image-manifest, windows-guest-seed]
surfaces: [cli, config, output]
security_boundary: none
native_validation:
  - {host: windows-hyperv, blocking: true, reason: "changes the Hyper-V Windows guest lifecycle end to end"}
  - {host: macos-qemu, blocking: true, reason: "qemu Windows path moves to the shared guest package and prepare-time tuning"}
tier2_gaps:
  - "Gen1 eval .vhd: IDE DVD seed discovery from an early-boot SYSTEM task, and a two-level .vhd chain (no Gen1 image on the validation host yet)"
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
   no longer contact the activation service). The guest is the untrusted side
   and the VM plus relay are the boundary, so in-guest AV adds no isolation.
6. **At most one prepare per driver cache at a time**, enforced by an OS file
   lock, with an atomic switch of the manifest's `active` pointer.
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

Same flags and pin/verify semantics as qemu (`--source`, `--sha256`,
`--accept-unpinned`, `--force`, `--name`), plus `--refresh` (re-prepare from the
already pinned eval image; no download). Steps:

1. Take the prepare lock (`<cache>/prepare.lock`, `LockFileEx`, exclusive,
   non-blocking). Held: exit 0 with "a prepare is already running (pid N,
   started T)".
2. Resolve and verify the pinned eval image (existing `evalimage.go` path; for
   now also `--image` for a local VHDX, recorded as unpinned unless `--sha256`).
3. Create `prepared/<name>-<timestamp>.vhdx` (or `.vhd` for Gen1) as a
   differencing child of the eval image.
4. Build a prepare seed: the OOBE unattend (oobeSystem pass from `winguest`,
   `LogonCount` 1), `warden-prepare.ps1` (tuning, task registration, grace
   report), and the `warden-run` task registration command.
5. Create the VM with **no network adapter** and COM1 on a host pipe (VM
   lifecycle privilege only; no Mount-VHD).
6. Boot. OOBE runs, the one-time FirstLogonCommand runs `warden-prepare.ps1`,
   which applies the tuning, registers the startup task, and powers off. If the
   tuning left a reboot pending (Defender removal), the script schedules itself
   to finish on the next boot and the host boots the VM again, until the script
   reports `[warden-prepare] done` on COM1 and the VM reaches Off.
7. The script also emits `[warden-prepare] grace-end=<RFC 3339>` and the license
   status on COM1; the host parses and records them.
8. Timeout with COM1 tail and the VM's last state as the error (UX-04). On
   failure the partial layer is deleted and `active` is unchanged.
9. Hash the prepared layer, mark it read-only, record it in the manifest, and
   swap `active` to it atomically (write temp + rename).

### ISO install stage (qemu now; any driver later)

`warden image prepare --driver qemu --os windows` keeps its ISO + virtio-win
inputs, but its result is a generalized base rather than a build-ready image:
the existing unattended install, then `sysprep /generalize /oobe /shutdown`.
The base is hashed, pinned and recorded with the sha256 of the ISO, the
virtio-win ISO and the install answer file. The prepared layer is then built
from it exactly as on Hyper-V. `--source` with an already installed,
generalized VHD/VHDX (converted with `qemu-img` for qemu) skips the install
stage on either driver.

### Manifest

`images/index.json` records generalized bases (path, sha256, source URL or
install inputs) and, per prepared layer: `path`, `sha256`, `size`,
`mtime`, `parent` (generalized base name + sha256), `generation`, `prepared_at`,
`grace_end`, `warden_version`, and `inputs` (sha256 of the unattend,
`warden-prepare.ps1`, and the tuning list). These map directly onto a
`vm-disk-chain/v1` prepared layer once the ledger environment types land.

### Per build

- Resolve `active`; check size, mtime and the child's recorded parent path
  (cheap). A moved cache dir shows up as a parent mismatch: fail with "re-run
  `warden image prepare --driver hyperv --refresh`" (checklist item 4).
- Grace check: under `image_min_grace` left, refuse (UX-04); base older than
  `image_refresh_after`, spawn a detached
  `warden image prepare --driver hyperv --refresh` (logs to
  `<cache>/prepare.log`) and continue on the current base. The lock makes
  repeated spawns harmless.
- Differencing child of the prepared base plus a `WARDEN` seed carrying only
  `warden-run.ps1` and `warden-io.exe`. The startup task finds the seed volume,
  runs `warden-run.ps1` as SYSTEM, and powers off, as on qemu.

### Retiring old prepared bases

After a refresh, an old prepared base is deleted only when no VM still has a
disk whose parent is that base (`Get-VM | Get-VMHardDiskDrive` ->
`Get-VHD ParentPath`). Checked at the end of each prepare and at build start;
deletion failures are logged, never fatal.

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
network adapter, so the only inputs to the prepared layer are the pinned eval
image and the seed, whose hashes are recorded. The prepared layer is a cached
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
- **Deferred (tier2_gaps):** Gen1 `.vhd` seed discovery and chain (items 2 and
  4 on Gen1), until a Gen1 eval image is on the validation host.

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

## Consolidation (merge round)

_To be filled in during the merge round._

## Rollback notes

_(none yet)_

## Archive summary

_To be filled in on merge._
