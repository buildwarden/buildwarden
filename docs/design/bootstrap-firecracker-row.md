---
feature: bootstrap-firecracker-row
status: draft
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

## Consolidation

## Rollback notes

Revert the commit; nothing else depends on the wording.

## Archive summary
