---
feature: native-refresh-scoped
status: archived
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-09, FLOW-13, FLOW-14]
concepts: [workflow, native-validation, scheduled-sessions, hosts]
related: [native-host-loop.md, firecracker-driver.md]
---

# A failed Hyper-V relay disk refresh skips only windows-hyperv work

## Problem

`native-tick.sh` rebuilds the cached Hyper-V relay boot disk whenever the
relay's sources move, and stopped the whole scheduled session if that rebuild
failed. On 2026-10-31 a merge touched a relay dependency (`relaycfg`), the
rebuild failed on `windows-desktop`, and every host that machine serves was
blocked, including `linux-kvm`, where the firecracker-driver spike waits.

## Prior art

- `native-host-loop.md` (archived): the scheduled per-machine session and its
  status file.

## Decisions

1. **Scope the failure to the hosts that use the asset.** A failed refresh
   marks `windows-hyperv` stale for the run; the session still starts, told to
   skip stale hosts, and works the machine's others. Rejected: stopping the
   session (blocks unrelated hosts); retrying the build (hides the failure).
2. **Keep the failure visible.** The status file's reason names the skipped
   host, and the digest says why.

## Questions

### Blocking

None.

### Assumed

None.

## Design

`native-tick.sh`: on a refresh failure, `stale=windows-hyperv`; the prompt to
the `workflow-native` skill adds "Stale hosts: ... skip their steps and gaps
this run and say why in the digest"; the run's recorded reason appends the
skipped host. The skill's `unattended` argument documents stale hosts: no step
or backlog entry for them, the other hosts as usual, and no claims at all when
nothing else is ready.

## Cross-driver impact

None.

## Validation plan

`bash -n` and shellcheck on `native-tick.sh`. First live use: the next
`windows-desktop` scheduled run after merge.

## Implementation plan

1. `native-tick.sh` and the `workflow-native` skill as above (FLOW-09, FLOW-14).
2. shellcheck (FLOW-17).

## Design review

- **r1, 2026-11-01: approved** (Jeff, attended session, FLOW-19), as
  presented in the session. Approved at the commit that adds this design.

## Consolidation

Branched from `main` at `66c1162`; nothing to reconcile.

## Rollback notes

## Archive summary

Implemented 2026-11-01. A failed Hyper-V relay boot disk refresh now skips only
`windows-hyperv` for that run: the session still starts with a stale-hosts
note, works the machine's other hosts, and records the skip in the status
file's reason and the digest. For future designs touching `scheduled-sessions`:
any per-host asset preparation in `native-tick.sh` should fail per host, not
per machine.
