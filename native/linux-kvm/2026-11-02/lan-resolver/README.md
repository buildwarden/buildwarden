# firecracker-distro-lan-resolver (2026-11-02, unattended)

Native run `host:linux-kvm/20261103T051002Z` on windows-desktop's WSL2 Ubuntu
22.04, against `main` at **1ef5547** (the merged Firecracker driver, PR #90).
Backlog entry `firecracker-distro-lan-resolver`: with `/etc/resolv.conf`
naming only one of the host's own LAN addresses, a firecracker build refuses in
preflight and gives the fix.

## How the host's resolver was swapped without changing the host

The entry's note says an attended session may not swap `/etc/resolv.conf`,
and the Never list keeps an unattended one from changing anything outside the
checkout. So each case ran in a **private mount namespace**:
`sudo unshare -m --propagation private`, a temporary file bind-mounted over
`/etc/resolv.conf`, then `runuser -u jeff -- env -i ... warden ...`. warden
runs as the unprivileged user with plain credentials and reads the
hard-coded `/etc/resolv.conf`, which inside the namespace is the temporary
file. The mount is invisible outside the namespace and ends with the process.
`run.sh` is the script, verbatim.

## Setup

- Host: kernel 6.18.54.1-microsoft-standard-WSL2-warden+, Firecracker v1.17.0,
  jeff in kvm, FORWARD policy DROP, firewalld inactive, eth0 172.19.128.47/20
  (MTU 1280), real resolvers 8.8.8.8 and 8.8.4.4.
- Binaries: `warden`, `warden-io-linux-amd64` and `warden-relay-linux-amd64`
  cross-compiled on the Windows host from main 1ef5547 (Go 1.26.3,
  `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`) into `.scratch/main/dist`.
- Boot assets beside the binary, from the spike-era tree in the WSL home
  (`~/bw`, not a git checkout): `firecracker-vmlinux` sha256 169495b7… (the
  Linux 6.1.191 kernel the attended validation used; `kernel.config` there
  matches main's, 418cf99c…), `firecracker-relay-initramfs.cpio.gz` sha256
  b5b657e0…, `firecracker-build-initramfs.cpio.gz` sha256 ebaf4177… (main's
  `tools/*/init` scripts match that tree's). The preflight refusal happens
  after the assets are only stat'ed; the control build boots them.
- One slot: `sudo warden setup --driver firecracker --slots 1`
  (`01-setup.log`).

## Results

| Case | resolv.conf in the namespace | Result | Log |
|---|---|---|---|
| baseline doctor | the host's real file | all ok (reflink warn, as on ext4); upstream DNS 8.8.8.8, 8.8.4.4 | `02-doctor-baseline.log` |
| 1, doctor | `nameserver 172.19.128.47` only | `FAIL upstream DNS for the relay`: "the host has only resolvers it cannot reach: 172.19.128.47"; fix "point the host's resolver at an off-host DNS server: list one as a nameserver in /etc/resolv.conf (under systemd-resolved, set DNS= in /etc/systemd/resolved.conf)"; exit 1 | `03-doctor-lan-only.log` |
| 1, build | same | `warden: build FAILED` in preflight with the same error and fix; exit 1; firecracker processes before 0, after 0 | `04-build-lan-only.log` |
| 2, doctor | `172.19.128.47` then `8.8.8.8` | ok, `upstream DNS for the relay: 8.8.8.8` (the LAN address excluded) | `05-doctor-lan-mixed.log` |
| control, build | the host's real file (copied) | direct-boot build SUCCEEDED: `hello from the build VM`, guest kernel 6.1.191 | `06-build-control.log` |
| control, inspect | | ENVIRONMENT firecracker linux/amd64, kernel and initrd `local`, 3 seed files; 6 records, 7 signatures valid, all channels closed | `07-inspect-control.log` |
| after | | host `/etc/resolv.conf` unchanged (8.8.8.8, 8.8.4.4); 0 mounts on it in the host namespace; no firecracker processes | `08-after.log` |

The control shows the namespace mechanism is not what refuses: the same
mechanism with the real resolvers runs a build end to end on main. It is also
the first linux-kvm build on `main` since the driver merged.

## Cleanup

`warden clean --driver firecracker` (nothing to clean; the 7.7 GB image cache
kept), `sudo sh tools/firecracker-vm/slots.sh down` (0 wfc interfaces left,
FORWARD DROP), the temporary resolv.conf files, build context and output dirs
removed (`09-cleanup.log`). WSL free disk afterwards: 944 GB.
