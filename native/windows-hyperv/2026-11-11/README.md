# windows-hyperv 2026-11-11: hyperv-relay-per-build-channel f6-validate-windows-hyperv

`host:windows-hyperv/20261111T231002Z`, unattended. Windows 11 Pro 26300 host,
Hyper-V Administrators, **not elevated**. Branch
`feat/hyperv-relay-per-build-channel` at **976ae9f** (`make -C
.scratch/hrpbc build`); main at 1173c5f for the baseline. Host state as left by
main: the privileged service installed (protocol 1), the old shared
`warden-dev` switch and `warden-dev-nat` (192.168.240.0/20, host
192.168.240.1), the cached relay boot VHDX at stamp 1, and the prepared base
`windows-server-2025-20261030T175627Z` whose evaluation grace ended
2026-11-09T17:57Z.

## Results

| Case | Result | Log |
|---|---|---|
| main doctor (baseline) | `Privileged service ... reachable`; `Network OK (fresh per-build via privileged service)`; not ready only for the expired base image | `main-1173c5f-doctor.log` |
| (1) branch doctor, stamp-1 VHDX | `BLOCKED: hyperv relay boot VHDX: link-channel stamp stale; rebuild it once: ... tools/relay-vm/hyperv/refresh-cache.sh`: as planned. Also `Privileged service ... not installed` (it is installed, protocol 1), `Network BLOCKED`, and the durable-switch refusal | `branch-976ae9f-doctor.log` |
| (1) branch build, stamp-1 VHDX | refused before any VM, but on the network, not the stamp: `the durable network has no build switches: run warden setup --driver hyperv (elevated)`. `refresh-cache.sh` is never named | `branch-976ae9f-build-refused.log` |
| (2)-(9) | not run: see below | |

## Failure (branch-caused, back to Implement)

The branch bumps `serviceProtocolVersion` 1 to 2 (`driver/hyperv/service.go`),
and the plan (Implementation plan, "The privileged service and the read-back
NAT") relies on "the existing mismatch refusal" naming `warden hyperv service
install`. That refusal never reaches the user: `serviceReachable`
(`driver/hyperv/service_windows.go`, unchanged from main) returns false on any
ping error, the version mismatch included. A host on which main's documented
non-elevated path works (the service installed) is told on upgrade that the
service is "not installed", and its builds are refused with the durable-switch
message instead of "reinstall the service". The Compatibility table's row
"durable network from an older setup ... elevated and service builds
unaffected" does not hold for a service installed by main.
Wanted: doctor reports a stale service as such (protocol N, reinstall with
`warden hyperv service install`), and the network strategy names that fix,
with tier-0 coverage of the mismatch through `serviceReachable`'s callers.

## Why cases 2-9 did not run (need Jeff, attended)

- The prepared base image's evaluation grace has ended: `warden image prepare
  --driver hyperv` (a fresh eval VHDX, ~11.7 GB download) before any build.
- Every build path on the branch needs elevation on this host: the service
  must be reinstalled at protocol 2 (elevated), durable builds need
  `warden setup --driver hyperv --nat-prefix 192.168.250.0/24 --host-ip
  192.168.250.1 --slots 2` (elevated, case 5), and case 2 is an elevated build.
- Cases 7 (host default-route DNS set to only 127.0.0.1) and 8 (host DNS
  pointed at an internal resolver) change the host's DNS settings, which an
  unattended session may not do.
- The relay boot VHDX was left at stamp 1 so the next attempt can repeat
  case 1 (refresh it with the branch's `refresh-cache.sh` after that).
