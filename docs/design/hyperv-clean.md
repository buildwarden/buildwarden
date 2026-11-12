---
feature: hyperv-clean
status: active
drivers: [hyperv]
seams: []
surfaces: [cli]
security_boundary: none
alters_relationships: false
design_review: skip
native_validation: []
tier2_gaps:
  - {host: windows-hyperv, what: "warden clean --driver hyperv against a --keep-vms build and a running build on a real host"}
rules: [TEST-01, UX-01, FLOW-01, ORCH-01]
rule_changes: []
concepts: [clean, keep-vms, hyperv, build-lock]
related: [firecracker-driver.md]
---

# warden clean for Hyper-V

## Problem

`warden clean --driver hyperv` falls through to the container path and fails
when no container runtime is running ("container runtime podman is installed
but not running"). It never looks at Hyper-V, so what a `--keep-vms` build or a
killed warden leaves behind (the build and relay VMs, the build's ephemeral
Private switch, the work dir with the VMs' overlays) has to be removed by hand
with `Remove-VM`, `Remove-VMSwitch` and a folder delete. The 2026-11-12 attended
Hyper-V validation of `hyperv-relay-per-build-channel` did that five times
(finding F4, `native/windows-hyperv/2026-11-12/attended/` on the `workflow`
branch).

## Prior art

`warden clean --driver firecracker` (`driver/firecracker/clean.go`,
firecracker-driver design) already cleans without a container runtime. Each
firecracker build holds a host lock `firecracker-build-<id>` for its run, and
clean removes a build's work dir only when it can take that lock. This design
takes that liveness rule unchanged.

## Decisions

1. **A per-build host lock is how clean tells live from left over.**
   `StartBuild` holds `hyperv-build-<id>` (`driver.HostLock`) from the build ID's
   creation to its return. Clean acts on a build only when `TryHostLock` gets it.
   Rejected: matching running `warden` processes, which the container path does
   with `ps aux`. Neither Windows nor the build's command line carries the build
   ID, and a pid check races with process reuse. Rejected: a VM's state (for
   example, removing only VMs that are Off). A `--keep-vms` relay is still
   Running, and a live build's VMs are Off for a moment before they start.
2. **`--keep-vms` leftovers are cleaned.** The lock is released when
   `StartBuild` returns, so a kept build is "no longer running" to clean, as with
   firecracker. Inspecting a kept build and then running clean is the expected
   order.
3. **Only build-ID-named resources.** Clean considers VMs named
   `warden-build-<id>` or `warden-relay-<id>`, and work dirs named
   `warden-hyperv-<id>-*`, where `<id>` is exactly 8 of `[a-z0-9]` (the shape of
   `driver.RandAlphaNum(8)`). The durable `warden-dev*` switches and NAT, prepare
   VMs (whose own lock and reaping cover them, `reap.go`) and anything else stay.
4. **Order: VMs, then the switch, then the work dir.** The switch carries the
   VMs, and the work dir holds their overlay disks. If a VM won't go, clean
   leaves that build's switch and work dir for the next run and reports the VM.
5. **No container runtime and no elevation.** Hyper-V Administrators can remove
   VMs and a Private switch (seen on the 2026-11-12 run), so clean needs neither
   the service nor a full Administrator.

## Questions

### Blocking

None.

### Assumed

None.

## Design

`hyperv.Clean(out, tempDir)`:

```
ids = build IDs from ListVMs("warden-build-"), ListVMs("warden-relay-"),
      and tempDir/warden-hyperv-<id>-* dirs           // a list error: clean nothing
for id in ids:
    if not TryHostLock("hyperv-build-" + id): continue   // running
    RemoveVM each of its VMs; any failure: report, skip the rest of this build
    TeardownNetwork("warden-" + id)                    // no-op on the durable strategy
    remove its work dirs
```

`warden clean` dispatches to it for `--driver hyperv` before resolving a
container runtime, as it does for firecracker. `StartBuild` takes the lock
right after it makes the build ID.

## Defaults and overrides

None. The temp dir is the one builds use (`[build] tmpdir`, else the OS temp
dir), resolved the same way as for firecracker.

## Cross-driver impact

Only hyperv changes. The container path is unchanged. Firecracker's clean is
unchanged, and this design copies its lock rule. qemu and vz have no clean of
their own, and this does not add one (ORCH-01: their leftovers are processes
that end with the warden that started them).

## Validation plan

- **Tier 0** (TEST-01, `driver/hyperv/clean_test.go`, untagged over a fake
  provisioner and a fake lock):
  - Finished builds' VMs, switch and work dirs are removed; a locked build's
    are kept.
  - A work dir with no VM is cleaned.
  - Prepare VMs, `warden-dev*` and firecracker work dirs are untouched.
  - A VM that won't go keeps its build's switch and work dir, and is reported.
  - A ListVMs error removes nothing.
  - `isBuildID` accepts only the build-ID shape.
- **Lint**: `make lint` (windows: `driver_windows.go` is tagged).
- **Tier 2** (not blocking; listed in `tier2_gaps`): on windows-hyperv, run
  `warden clean --driver hyperv` after a `--keep-vms` build (everything goes),
  and again during a running build (that build is untouched). The next attended
  Hyper-V session covers it.

## Implementation plan

1. `driver/hyperv/clean.go`: `buildLockName`, `isBuildID`, `Clean` and its
   testable core over the `cleaner` subset of `Provisioner` (TEST-01).
2. `driver/hyperv/driver_windows.go`: `StartBuild` holds the build lock.
3. `cmd/warden/clean.go`: the hyperv branch and its help text (UX-01).
4. `docs/book/src/guide/cli.md` (`warden clean`) and `drivers.md` (Hyper-V
   Examples, `--keep-vms`): what is removed and what stays (UX-01).
5. `driver/hyperv/clean_test.go`: the tier-0 cases above.

## Design review

Skipped (FLOW-19): no seams, no relationship change, no security boundary, no
blocking native validation, no assumed questions, no rule changes.
