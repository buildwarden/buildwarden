---
feature: bootstrap-firecracker-row
status: archived
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [BOOT-07, FLOW-14]
concepts: [bootstrap-model, firecracker, rules]
related: [firecracker-driver.md]
---

# bootstrap.md: the Firecracker mapping row and BOOT-07 surface list

## Problem

`firecracker-driver` adds a fifth driver and moves qemu's Linux seed code to
`driver/linuxguest/`. Its design proposed two changes to
`docs/development/bootstrap.md` (its "Proposed standards change"), and design
review r1 (2026-10-31) accepted the Bootstrap comparison and moved the edit to
this process item, since `docs/development/` changes only through the human
(FLOW-14). Until it lands, `bootstrap.md` has no firecracker row and its
BOOT-07 surface list names `driver/qemu/cloudinit.go`, which the driver branch
deletes, while `tools/tier1/rules.yaml` on that branch already lists the new
surface. The driver's tier-2 reports carry this as SHOULD finding 6.

## Prior art

- `firecracker-driver.md`: "Proposed standards change (FLOW-14)" and
  "Bootstrap comparison", accepted at its design review r1; its tier-2 findings
  6 and 14 (`setup.go` and `posture.go` belong on the surface).
- `relay05-specs-wording.md`: the same shape, a rule wording change split out
  of a feature and landed after it.

## Decisions

1. **Land after `firecracker-driver` merges,** so `bootstrap.md` never names
   files that are not on `main`. Implement can run as soon as this design is
   approved; the PR waits for the driver's merge (Jeff merges the driver
   first). Rejected: folding the edit into the driver's PR (FLOW-14).
2. **The two accepted changes, plus the surface paths review added,** and
   nothing else in the model: the per-driver mapping row exactly as the
   driver's Bootstrap comparison states it; the BOOT-07 surface list made to
   match the driver branch's `rules.yaml`.
3. **Retire the `docs/development/README.md` entry** "Feature — Firecracker
   driver", which still points at the superseded `firecracker-driver-plan.md`
   (the driver's consolidation finding). Once the driver merges its design is
   archived, so the entry is removed rather than repointed.
4. **Not in this item:** listing the `firecracker` workflow under TEST-02's
   "Where each tier runs today" in `testing.md`. That waits until the job has
   passed and is relied on (the driver's consolidation proposal), which is a
   later, separate change.

## Questions

### Blocking

None.

### Assumed

None.

## Design

In `docs/development/bootstrap.md`, "Per-driver mapping", a row after
`qemu / Linux`:

| Driver / guest | (1) Isolation | (2) Executor delivery | (3) Start trigger |
|---|---|---|---|
| firecracker / Linux | Topology: the build VM's only NIC is a TAP on a per-slot host bridge whose only other port is the relay VM; the bridge has no address and drops every frame bound for the host | cloud-init `CIDATA` ISO per build as a read-only virtio-blk drive (disk path), or `warden-io` appended per build to a launcher-only initramfs (direct boot) | cloud-init `runcmd` (disk path); the launcher's exec of `warden-io initialize` (direct boot) |

The deviation register gains nothing.

In BOOT-07's surface paragraph, the driver list becomes:
`driver/container/container.go`, `driver/container/images*`,
`cmd/warden-pause/`, `driver/linuxguest/`, `driver/winguest/`,
`driver/hyperv/winseed*.go`, `driver/vz/image.go`, and the firecracker
isolation, delivery and start files (`driver/firecracker/slots*.go`,
`setup.go`, `posture.go`, `vm*.go`, `deliver_linux.go`) and
`tools/firecracker-vm/`. This is the `bootstrap.surface` list in
`tools/tier1/rules.yaml` on the driver branch, word for word.

In `docs/development/README.md`: remove "Feature — Firecracker driver".

## Cross-driver impact

None: documentation only. The qemu entry changes because its file moved, not
because qemu's model changed.

## Bootstrap comparison

This item writes down the model change the driver's Bootstrap comparison
already made and review accepted; it changes no invariant and adds no
deviation.

## Security note

None (documentation).

## Validation plan

`make lint` (tier 1's BOOT-07 coupling check: this diff changes
`bootstrap.md` with a design whose `rules` cites BOOT-07) and `make test`.
After both merge, tier 1 on `main` no longer gives the BOOT-07 hint for the
driver's files.

## Implementation plan

1. Edit `bootstrap.md` and `docs/development/README.md` as above.
2. `make lint` and `make test`; archive this design (FLOW-12) and open the PR,
   noting it merges after `firecracker-driver`.

## Design review

- **r1, 2026-11-02, Jeff: approved** as drafted, approved at a01f7ae.
  Decisions confirmed: land after `firecracker-driver` merges (Jeff merges the
  driver first); remove the `docs/development/README.md` entry rather than
  repoint it; the TEST-02 `testing.md` listing of the `firecracker` job stays
  out until that job has passed and is relied on.

## Consolidation

`origin/main` at `1ef5547` (#90, the Firecracker driver) merged at `ca812fb`
before the edit, so the driver this item depends on (Decision 1) is on `main`.

- **Landed since the fork:** #90. `bootstrap.md`'s surface list now matches
  `bootstrap.surface` in `tools/tier1/rules.yaml` on `main`;
  `firecracker-driver.md` is archived there.
- **Left alone, outside this design:** `firecracker-driver-plan.md` stays on
  `main` and other designs cite it in `related:`; `hyperv-driver-plan.md` still
  names `cloudinit.go` as history; the TEST-02 `testing.md` listing of the
  `firecracker` job (Decision 4).
- **FLOW-15 sweep:** no Go code changed, so no dead code or duplication.
  `overview.md` does not list per-driver bootstrap files and is unchanged.

## Rollback notes

Revert the commit; nothing else depends on the wording.

## Archive summary

Approved at design review r1 (2026-11-02); the PR to `main` carries this
design.

- **Built.** `docs/development/bootstrap.md`: a `firecracker / Linux` row in
  "Per-driver mapping" (topology isolation through a per-slot, addressless host
  bridge; `CIDATA` virtio-blk drive or a per-build initramfs append; cloud-init
  `runcmd` or the launcher's exec of `warden-io initialize`), and BOOT-07's
  surface list with `driver/linuxguest/` in place of `driver/qemu/cloudinit.go`
  plus the firecracker `slots*.go`, `setup.go`, `posture.go`, `vm*.go`,
  `deliver_linux.go` and `tools/firecracker-vm/`.
  `docs/development/README.md`: the "Feature — Firecracker driver" entry
  removed.
- **Decisions taken:** land after the driver merged; the row word for word from
  the driver's accepted Bootstrap comparison; the surface list word for word
  from `tools/tier1/rules.yaml`; remove the README entry rather than repoint it.
- **Rejected:** folding the edit into the driver's PR (FLOW-14); listing the
  `firecracker` job under TEST-02 now.
- **For a future design touching `bootstrap-model`, `firecracker` or
  `rules`:** BOOT-07's prose list and `bootstrap.surface` in `rules.yaml` must
  stay identical; a driver that moves or adds isolation, delivery or start
  files changes both, and the prose half goes through the human (FLOW-14),
  usually as a process item like this one. Open: the TEST-02 `testing.md`
  listing of the `firecracker` job, once that job is relied on.
