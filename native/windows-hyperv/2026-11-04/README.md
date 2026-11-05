# windows-hyperv 2026-11-04: build-link-subnet f6-validate-windows-hyperv

`host:windows-hyperv/20261105T051002Z`, unattended. Windows 11 Pro 26300 host,
Hyper-V Administrators (not elevated, network via the privileged service),
prepared base windows-server-2025-20261030T175627Z (eval grace ends
2026-11-09), durable switches `warden-dev` (Private) and `warden-dev-nat`
(Internal, NAT 192.168.240.0/20, host 192.168.240.1). Branch
`feat/build-link-subnet` at **a68ac2c** (`go build -C .scratch/build-link-subnet
-o dist/warden.exe ./cmd/warden`, run from the worktree so the cross-built
guest binaries come from the branch); main at 4c8f9dc for the baseline. Every
windows-hyperv case in the Validation plan passes.

`scripts/build.ps1` is the guest script: `ipconfig /all`, `route print`,
`Get-NetIPAddress` origins, the Dhcp-Client log, `C:\warden\warden-run.log`,
HTTPS through the relay (Invoke-WebRequest and curl.exe without
`--ssl-no-revoke`), HTTP to the link's relay address, a direct TCP connect to
1.1.1.1:8080, the relay CA count, and a 45 s pause for the host-side DHCP
probe. `scripts/relay-serial.ps1` attaches to the relay VM's COM1 pipe once it
is 5 s up (the driver does not record the relay console); `scripts/wait-relay.ps1`
gates the probe on the relay VM's uptime; `scripts/{main,probe_*}.go` is the
DISCOVER probe (`go run -C .scratch/dhcpprobe . -src 192.168.240.1`);
`scripts/Makefile` launched the branch's `refresh-cache.sh` and the forged-stamp
build (the native allowlist runs `make` and `.scratch/*/dist/warden*`, not
`.scratch/*/tools/*` or an env-prefixed command).

## The cached relay boot VHDX

The scheduled wrapper had refreshed it from main 353e8a7 at 21:10 (no
`relay-boot.link-channel`), so the pre-change asset was in place for case 1.
The branch's `refresh-cache.sh` (`logs/refresh.log`) rebuilt it from relay
source e00c7bd068b7 (the branch) and installed the stamp; `--check` then said
current. The main-built disk moved to `relay-boot.vhdx.prev`. A copy of the
old disk under `.scratch/hv/old/` served the forged-stamp case.

## Results

| Case | Result | Log |
|---|---|---|
| main 4c8f9dc build (baseline, old VHDX) | SUCCEEDED; guest static 10.0.0.2/30 via 10.0.0.1 (Manual), HTTPS 200/200; `http://10.0.0.1/` through the proxy hung 15 s and left an UNCLOSED channel (main's vm-mode relay does not block dials into the link); 22 signatures valid | `main-build.log`, `main-inspect.log` |
| (1) branch doctor, old VHDX | `BLOCKED: hyperv relay boot VHDX: link-channel stamp missing; rebuild it once: … tools/relay-vm/hyperv/refresh-cache.sh`, exit 1 | `../doctor-old-vhdx.log` |
| (1) branch build, old VHDX | refused before any VM: `predates the configurable build link and must be rebuilt once (no link-channel stamp at …relay-boot.link-channel)`, the same fix | `../old-vhdx-refused.log` |
| old VHDX under a forged stamp | the old relay booted and refused the claim: `the relay predates the build link in its claim (… HTTP 400: invalid claim: json: unknown field "build_link")`, mapped to the refresh fix; VMs and the per-build switch torn down | `../old-vhdx-refused.log` |
| the one-time refresh | `refresh-cache.sh` from the branch tree: VHDX rebuilt (114 MB), `relay-boot.link-channel` installed; doctor `hyperv relay boot VHDX: link-channel stamp current`, exit 0 | `refresh.log`, `doctor-after-refresh.log` |
| (2) default link build | SUCCEEDED. Relay console (hv_netvsc): `relay: build-if eth0 100.127.255.253/30 from the claim; DHCP offers 100.127.255.254`, then `DHCP OFFER/ACK 100.127.255.254 to 00:15:5d:cc:b1:eb` (the guest's NIC). Guest: `IPv4 Address 100.127.255.254`, mask 255.255.255.252, gateway and DHCP server 100.127.255.253, `DHCP Enabled: Yes`, lease obtained 05:30:38 and expiring in 2162 (infinite), `PrefixOrigin Dhcp`; warden-run.log then ran `initialize --gateway=100.127.255.253 --ip=100.127.255.254/30`: the address and route equal the seed's after taking the lease first. HTTPS 200 and 200 (16337 bytes); `http://100.127.255.253/` through the proxy: relay `upstream error: blocked: connection to 100.127.255.253 denied`, guest 502; direct egress not connected; 1 warden root. `inspect --verify-disks`: vm-disk-chain/v1 verified, both disks [ok], 19 signatures valid, all channels closed | `default-build.log`, `default-relay-serial.log`, `default-inspect.log` |
| (3) `--build-link 100.126.255.252/30` build, no refresh | SUCCEEDED the same way at 100.126.255.254 via 100.126.255.253 (lease first, `PrefixOrigin Dhcp`, then initialize with the same values); `http://100.126.255.253/` refused (502); verified, 19 signatures valid, all channels closed | `alt-build.log`, `alt-relay-serial.log`, `alt-inspect.log` |
| (3) `[vm] build_link` in `./warden.toml` | doctor `build link 100.126.255.252/30 (warden.toml)`, stamp current; `--build-link 100.125.255.252/30` wins over the key | `doctor-toml.log` |
| (4) `--build-link` overlapping BuildWarden ranges | `192.168.240.0/30` (hyperv default NAT), `100.64.88.0/30` (firecracker upstream), `10.0.2.0/30` (qemu user-mode): each refused at startup with the range named, before doctor runs anything | `refuse-hyperv-nat.log`, `refuse-fc-upstream.log`, `refuse-qemu-usermode.log` |
| `--build-link 172.19.128.0/30` (holds the WSL vEthernet's 172.19.128.1) | doctor `build link: BLOCKED: host interface address 172.19.128.1/20 is inside the link`, exit 1 | `refuse-host-address.log` |
| `--build-link 172.19.140.0/30` (inside a host network, no address) | `warning: host network 172.19.128.1/20 contains the link (only the link's four addresses are shadowed)` | `warn-host-network.log` |
| misaligned `100.126.255.253/30`; `--build-link` on `--driver container` | `is not a network address; did you mean 100.126.255.252/30?`; `--build-link requires a VM driver` | `refuse-misaligned.log`, `refuse-container.log` |
| `setup --driver hyperv --nat-prefix 100.127.255.0/24` | refused before any change: `overlaps the build link 100.127.255.252/30`, fix names another `--nat-prefix` or `[vm] build_link`; with `warden.toml` setting 100.126.255.252/30, `--nat-prefix 100.126.255.0/24` refused against the key | `refuse-natprefix-default.log`, `setup-natprefix-toml.log` |
| (a14 d) DISCOVER on the relay VM's upstream network | during the configured-link build, with `warden-relay-ik6gq0zc` up 76 s on `warden-dev-nat` (its NIC2) and `warden-ik6gq0zc` (NIC1), two DISCOVERs from the host's 192.168.240.1 (UDP :68, SO_REUSEADDR, broadcast to :67): no BOOTREPLY within 10 s each; the relay's console shows no offer but the guest's | `alt-build.log`, `alt-relay-serial.log`, the probe output in the digest |

The host's `warden.toml` was a temporary file at the checkout root for the
key cases and was removed (`git status` clean).

## Findings (not blocking)

1. **On the branch the Windows guest stays DHCP-configured.** On main the
   guest NIC shows `DHCP Enabled: No`, `PrefixOrigin Manual` (warden-io's
   static address). On the branch it shows `DHCP Enabled: Yes`, `PrefixOrigin
   Dhcp`, the relay's infinite lease, and `initialize --gateway --ip` reports
   no error. The address and route are the seed's either way, which is what
   the design asks for; whether `warden-io initialize` should still switch the
   NIC to static once a lease holds the same address is a question for the
   feature (a NIC left on DHCP would re-request at lease events, which an
   infinite lease never raises).
2. **A main-built relay VHDX keeps the branch's stamp.** main's
   `refresh-cache.sh` does not touch `relay-boot.link-channel`, so when the
   wrapper next refreshes the cache from main the stamp stays and a branch
   binary accepts the disk; the old relay then refuses the claim and the
   build fails with the refresh fix (the forged-stamp path above). Harmless,
   and gone once the branch merges.
3. **The driver's stdout to a file is buffered until the build ends**, so a
   log-marker gate for timing the probe did not work; the VM-uptime gate did.
