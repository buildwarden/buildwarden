# hyperv-relay-per-build-channel, f6-validate-windows-hyperv, attended (2026-11-12)

Worker `host:windows-hyperv/20261112T183259Z`, windows-desktop (Windows 11 Pro 10.0.26300), with Jeff
attending. Branch `feat/hyperv-relay-per-build-channel` at 59d7440, built in `.scratch/hrpbc` (`make build`,
plus `dist/warden-io-windows-amd64.exe` built beside it, see finding F5). Claude ran non-elevated (Hyper-V
Administrators); Jeff ran the elevated steps (service reinstall, `setup`, `Remove-NetNat`, one elevated build).

Setup done in this run: base re-prepared (`windows-server-2025-20261112T183421Z.vhdx`, grace to 2026-11-22);
the privileged service reinstalled at protocol 2 from `%LOCALAPPDATA%\warden\validate\warden.exe` (the
branch build; reinstall from main once this merges); the relay boot VHDX rebuilt to stamp 2; durable
network moved to `192.168.250.0/24`, host `.1`, slots `warden-dev-0`/`-1`.

Plan change (Jeff, attended): case 7's 127.0.0.1-only host-DNS refusal and case 8's "host DNS on an
internal resolver" were dropped, since changing the host's DNS can cut the session off; tier-0 covers them.

| Case | Result | Evidence (logs/) |
|---|---|---|
| 0 stale service | PASS. Before: stale protocol 1 vs 2, BLOCKED, Not ready, uninstall-then-install (11:10Z run). After Jeff's reinstall: `Privileged service ... reachable`, Network OK | `case0-doctor-after.txt` |
| 1 upgrade | PASS. Stamp-1 build refused before any VM naming `refresh-cache.sh`; after it `WARDEN_LINK_CHANNEL=2`, doctor "stamp current" | `case1-refused.log`, `case1-refresh.log`, `case1-doctor-after.txt` |
| 2 default build | PASS via the service and elevated (`case5e`). Ledger verifies. Seed: `nat_ip`, `nat_prefix`, `nat_gw`, token, no resolver. Console: `upstream resolvers: 8.8.8.8, 8.8.4.4 (claim)`, no token value anywhere. `initialize --gateway=100.127.255.253`, no `--ip`; guest NIC `Dhcp` at 100.127.255.254. Init's `nat-if` line was not captured (the driver does not record the relay console; an outside reader attaches too late), the address is shown by the seed and VM Notes | `case2-*.log`, `case9b-build.log` |
| 3 claim auth | PASS on a relay booted by hand from a kept build's seed (an in-build probe never landed in the pre-claim window): claim and tunnel, no token and wrong token, all 401; right token gets past auth (400 on the empty body) | `case3-hand-probe.txt` |
| 4 concurrency | **FAIL** (F1). Allocator PASS: builds took .3 beside a held .2. But 2 of 3 simultaneously started pairs lost one build in `buildSeed`: `IMAPI2FS.MsftFileSystemImage` COM creation fails 0xC0AAB138 | `case4*.log`, `case4-monitor*.txt` |
| 5 durable | PASS. Pre-setup durable build refused before any VM naming `warden setup`; setup refused on the 240/20 NAT naming `Remove-NetNat`, then made both slots and retired `warden-dev`; doctor prints prefix, host IP, two free slots; non-elevated build on `warden-dev-0`, relay 192.168.250.2; elevated build on its own `warden-ubbeggpv` through the same NAT | `case5-*` |
| 6 durable concurrency | PASS. A on slot 0, B on slot 1 at once; from each build VM the `.253` neighbour is its own relay's MAC (A 00-15-5D-CC-B1-2B, B ...2D) and a DHCP DISCOVER gets exactly one reply; third build refused before any VM naming both holders and `--slots`; a `--keep-vms` slot reads in use in doctor until removed | `case6*` |
| 7 refusals | PASS (with F3): `build_link` in the NAT refused at build and in doctor (default and durable NAT); `[vm] dns = [192.168.240.1]` refused naming it; a relay with no seed prints `init: stopped: no relay seed (WARDENRELAY): no CD after 10 s` and serves nothing on :8301 | `case7-*` |
| 8 resolvers | PASS (`[vm] dns` halves). 208.67.222.222: doctor and console list only it, the guest resolves `myip.opendns.com` (an OpenDNS-only name); 192.0.2.1: lookups fail, no fallback | `case8*` |
| 9 no lease | PASS with the branch's warden-io: `relay at 100.127.255.253 not reachable within 60s: Ethernet has 169.254.45.180 (WellKnown, no DHCP lease from the relay)`, no static address, powered off | `case9b-*` |

## Findings

- **F1 (blocks case 4).** Concurrent Hyper-V builds race on seed ISO creation: one fails creating the
  IMAPI2FS COM object (0xC0AAB138), 2 of 3 simultaneous pairs. `winseed_windows.go` is unchanged from main,
  but main never ran two ephemeral builds at once and this branch builds two ISOs per build. Fix: serialize
  ISO creation across processes (a named mutex) or retry that HRESULT, with a test.
- **F2.** Doctor reports an installed but stopped privileged service as "not installed".
- **F3.** The `build_link` overlap refusal names "the hyperv driver's default NAT (192.168.240.0/20)" when the
  host's NAT is 192.168.250.0/24 (doctor reads the right one); the check appears to use the default prefix.
- **F4.** `warden clean --driver hyperv` falls through to the container runtime instead of removing Hyper-V
  leftovers (kept VMs, ephemeral switches).
- **F5 (pre-existing).** `driver.GuestBinary`'s cache (`%LOCALAPPDATA%\warden\bin`) is shared by every
  checkout and judged only by mtime: another checkout's newer warden rebuilt `warden-io-windows-amd64.exe`
  from main mid-run, and this branch's builds used it. Pinned by building it beside `dist/warden`.
