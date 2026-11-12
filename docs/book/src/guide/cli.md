# CLI Reference

## warden build

```
warden build [path] [flags]
```

Run an audited build with full network recording. The `path` argument can be a
directory (default: current directory), a specific Dockerfile, or, for VM
drivers, a `build.sh` / `build.ps1` script. All network traffic during the
build is recorded to a cryptographically signed ledger.

`build` and `shell` share one flag set. Every driver accepts it; each driver
rejects only the flags it genuinely cannot use.

**Flags:**
- `-o, --output <dir>` — Output directory (default: `warden-output`)
- `--capture <mode>` — Capture payloads: none, headers, bodies, all
- `--no-compress` — Disable zstd compression of the ledger and CA certificate
- `--timeout <duration>` — Maximum build duration (e.g. "10m", "1h"); all drivers
- `--script <path>` — Build script to run (VM drivers)
- `--image <path>` — Base disk image for the build VM (VM drivers)
- `--guest-os <os>` — Guest OS for VM drivers: `linux` (default) or `windows`
- `--cpus <n>` — Build VM vCPUs (VM drivers; default per driver)
- `--memory <MiB>` — Build VM memory in MiB (VM drivers; default per driver)
- `--network <name>` — Pre-provisioned network to reuse (hyperv: the durable network from `warden setup`, whose build switches are `<name>-0` and up)
- `--keep-vms` — Leave VMs, disks and the work dir in place for inspection (VM drivers)
- `--tmpdir <dir>` — Directory each build's private work dir is created in (default: the OS temp dir, which honours `TMPDIR`)
- `--build-link <cidr>` — Advanced: the VM drivers' relay-to-build link, a `/30`
  (default: `[vm] build_link`, then `100.127.255.252/30`; see
  [Configuration](configuration.md#the-build-link-vm-build_link---build-link))
- `-q, --quiet` — Do not echo the guest build console (still captured to `build-output.log`)

The container driver refuses the VM-only flags (`--script`, `--image`, `--cpus`,
`--memory`, `--keep-vms`, `--build-link`); `--network` is only used by `--driver hyperv`
(firecracker builds take a network slot made by `warden setup`).
`--guest-os windows` is supported by `--driver qemu` and `--driver hyperv`;
`--driver firecracker` runs Linux guests only.

Defaults for `--cpus`, `--memory` and `--network` come from the `[vm]` section
of `warden.toml`. Per-driver VM defaults: qemu 4 CPU / 4096 MiB, vz 4 / 8192,
hyperv 4 / 4096, firecracker 4 / 4096 (its relay VM: 2 / 256).

With `--driver firecracker`, `--keep-vms` leaves both VMs running and their
network slot held, and prints their pids; `warden clean --driver firecracker`
stops them.

Every build stages its working files (overlay disks, seeds, the container
driver's `/.warden` tree and relay image context) in its own fresh directory,
`warden-<driver>-XXXXXXXX`, created like `mktemp -d` under `--tmpdir` (or
`[build] tmpdir`). Nothing is written into the build context, so concurrent
builds of the same directory do not interfere. With a VM-backed container
runtime (finch, Docker Desktop, podman machine) the directory must be one the
runtime's VM shares; the macOS temp dir is shared by all three.

**Exit status:**

| Exit code | Meaning |
|-----------|---------|
| `0` | The build succeeded |
| the build's own code | The build script exited non-zero (`warden: build FAILED (exit code N)`) |
| `124` | `--timeout` expired |
| `130` | Interrupted (Ctrl-C) |
| `1` | Any other failure. A known cause (missing root or Administrator, unprepared image, absent tool, bad flag) is printed with `To fix: <what to do>`; only an unexplained failure carries the issue link |

`warden shell` exits with the shell's own exit status.

**Example:**
```sh
# Container driver (default)
warden build ./my-project -o ./audit-results

# QEMU driver with a cloud image
warden build --driver qemu --image ubuntu-24.04.qcow2 ./my-project

# VZ driver with a macOS build script
warden build --driver vz --script build.sh ./my-project

# Windows build in a sized VM
warden build --driver qemu --guest-os windows --cpus 8 --memory 16384 ./win-project
```

## warden shell

```
warden shell [path] [flags]
```

Open an interactive shell in the audited build environment. Useful for
debugging network behavior or exploring what a build fetches. Same flags as
`build` (minus `-q/--quiet`). Drivers that cannot attach an interactive session
say so.

```sh
warden shell ./my-project
# Now inside the isolated environment — try curl, apt-get, etc.
# All traffic is recorded to the ledger.
```

## warden inspect

```
warden inspect <path> [flags]
```

Verify and display a build ledger.

**Arguments:**
- `path` — Ledger file or output directory (auto-finds `ledger.zst` or `ledger`)

**Flags:**
- `--json` — Output as JSON
- `--verbosity <n>` — Detail level: 0=compact, 1=tree, 2=full
- `--extract <dir>` — Extract captured payloads to a directory
- `--verify-disks` — Re-hash each VM disk layer the environment record names
  (reads every byte of every base image)

Inspect also decodes the build environment record and checks
`environment.json` against it; a mismatch fails verification. For VM builds it
lists the disk layers with their hashes and provenance (downloaded and pinned,
prepared by warden, or local).

```sh
warden inspect warden-output --json | jq '.summary'
warden inspect warden-output --json | jq '.environment'
warden inspect --verify-disks warden-output
```

## warden image

Prepare and list build-guest base images for the VM drivers.

### warden image prepare

```
warden image prepare [source] [flags]
```

Acquire, verify and ready a base image for a VM driver. One verb covers every
driver; it replaces the old `image restore`, `image fetch`, and vz-only
`image prepare`. The driver comes from `--driver` (or `warden.toml`); `--os`
defaults per driver (vz: macos, qemu and firecracker: linux, hyperv: windows). `[source]` (or
`--source`) is a local path or URL.

| Driver / OS | Source | Behavior |
|---|---|---|
| `vz` / macos | IPSW URL, a local `.ipsw` file, or a prepared image directory in `~/.cache/warden/images` (or its `disk.img`). No source: the active image | Run under `sudo`. With no source, refreshes the active image (the one builds use without `--image`) in place, with no IPSW download, and warns when a newer macOS is available; with no image prepared yet, restores the latest supported IPSW. An IPSW whose image is already prepared, and a prepared image directory, are refreshed in place; another IPSW is restored. A local `.ipsw` is read in place and kept; a downloaded IPSW is deleted from the cache once its image is prepared. The result becomes the active image. `--update` moves to the latest supported IPSW |
| `qemu` / linux | cloud image ref (`alpine:3.21`, `ubuntu:24.04`, `debian:bookworm`) or qcow2 | Downloads and verifies the cloud image |
| `firecracker` / linux | cloud image ref (default `ubuntu:24.04`), or a local qcow2 or raw image | Downloads the cloud image if needed (as qemu does), checks it with `qemu-img info` (a backing file, an external data file or another format is refused), and converts it once to the raw image builds copy, in `~/.cache/warden/firecracker/images` |
| `qemu` / windows | Windows install ISO (required the first time) | Unattended install ending in `sysprep /generalize` (skipped if already installed unless `--force`), then prepares the layer builds boot from. With no source, re-prepares |
| `hyperv` / windows | Windows Server evaluation VHDX: URL, or a local file registered in place (default: `[vm] windows_image`, else Microsoft's current Server 2025 evaluation download) | Fetches or registers the base, then prepares the layer builds boot from. With no source it is the refresh: re-checks a downloaded base for new media and re-prepares |

**Flags:**
- `--os <os>` — Guest OS: macos, linux or windows (default per driver)
- `--source <path|url>` — Image source (same as `[source]`)
- `--sha256 <hash>` — Expected SHA256 of the image; verified (the digest of what
  was used is always recorded)
- `--force` — Re-acquire even if a valid cached copy exists
- `--name <n>` — Registry name of the image (hyperv; default windows-server-2025)
- `--edition <e>` — Windows edition / install.wim image name (qemu windows)
- `--virtio <v>` — virtio-win driver ISO path or URL (qemu windows)
- `--arch <a>` — Guest arch: arm64 or amd64 (default host arch)
- `--keep-vms` — Leave the prepare VM and its disk for inspection (hyperv)
- `--update` — Move to the newest supported OS release: restore it, or refresh
  its image in place when it is already prepared, and make it active (vz only;
  takes no source; refused on other drivers)

```sh
sudo warden image prepare --driver vz
sudo warden image prepare --driver vz --update
sudo warden image prepare --driver vz ~/Downloads/UniversalMac_27.2_26B123_Restore.ipsw
sudo warden image prepare --driver vz ~/.cache/warden/images/macos-90a1b1f94f64b00a
warden image prepare --driver qemu ubuntu:24.04
warden image prepare --driver firecracker ubuntu:24.04
warden image prepare --driver qemu --os windows --source Win11_Arm64.iso
warden image prepare --driver hyperv --source https://.../server2025.vhd
```

### warden image list

```
warden image list
```

List prepared base images for every driver. Every prepared vz image is
listed, the active one (used by builds without `--image`) first and marked
`active`, the others `older`, each with the IPSW it was restored from when
that was recorded.

```sh
warden image list
# vz      macos    /Users/me/.cache/warden/images/macos-a139c218a2010753  active  UniversalMac_27.2_26B123_Restore.ipsw
# vz      macos    /Users/me/.cache/warden/images/macos-90a1b1f94f64b00a  older   UniversalMac_27.0.1_26A100_Restore.ipsw
# qemu    linux    /Users/me/.cache/warden/images/alpine.qcow2
# firecracker linux /home/me/.cache/warden/firecracker/images/ubuntu_24_04-1a2b3c4d5e6f.raw
# hyperv  windows  base     C:\...\server2025.vhdx (pinned)
# hyperv  windows  prepared windows-server-2025-20261020T193301Z.vhdx, 2.0d old,
#                           evaluation grace ends 2026-10-30 19:33Z (8.0d left)
```

### warden image verify

```
warden image verify [--driver <driver>]
```

Re-hash, in full, every base-image file a VM driver has recorded in its layer
cache: the hashes builds write into the ledger's environment record. Fetch and
prepare record each image's hash and provenance once; builds reuse it while the
file's size and modification time are unchanged. `verify` reads every byte
instead, re-records a changed file as local (its download/prepare provenance no
longer applies), drops missing files, and exits 1 if anything drifted. Without
`--driver` it checks every VM driver's cache on this host.

```sh
warden image verify --driver hyperv
# hyperv  ok       C:\...\images\windows-server-2025.vhd
```

## warden doctor

```
warden doctor --driver <name> [--build-link <cidr>]
```

Read-only preflight for the selected driver. Checks a container runtime
(container), `qemu-system-*` + `qemu-img` and relay VM assets (qemu), Apple
Silicon macOS and the virtualization entitlement (vz), or the hypervisor and
management stack and the privilege tier this process holds (hyperv). For
firecracker it checks `/dev/kvm`, the `firecracker` binary (v1.17.0 or later),
the boot assets, the disk-image tools (`qemu-img`, `genisoimage`; a warning
only), the network slots owned by this user and their sysctls, firewalld, an
off-host upstream resolver, reflink support on the cache filesystem (a warning)
and VMMs left on a slot with no warden (a warning). Run as root (`sudo warden
doctor --driver firecracker`) it also checks the `warden_fc` bridge table, that
each `WARDEN-FC` jump is first in its chain, and the FORWARD policy, and counts
the slots, `/dev/kvm` access (kvm group membership) and leftover VMMs of the
user who ran sudo, skipping the reflink probe so nothing is written into that
user's cache.

Every VM driver's doctor also prints its build link and where it came from
(`build link 100.127.255.252/30 (default)`, or `warden.toml`, `config.toml`,
`--build-link`), whether a host interface overlaps it (an address inside the
link is BLOCKED, since every build would refuse it; a host network that only
contains it is a warning), and the state of each launcher asset's
link-channel stamp: `current`, or `missing`/`stale` with the one-time rebuild
(the qemu and Firecracker direct-boot initramfs, the Hyper-V relay boot VHDX,
the vz prepared image). A missing or stale stamp on the Hyper-V relay boot
VHDX or the vz prepared image is BLOCKED and fails doctor, since every build
of that driver uses the asset; on the direct-boot initramfs, which only direct
boot uses, it is reported only. `--build-link` checks a value before you configure it;
the container driver refuses it.

Every VM driver's doctor also prints the relay's upstream resolvers and their
source (`[vm] dns`, or the host source it read them from), or the refusal a
build would get, which is BLOCKED and fails doctor. The hyperv doctor adds a
`Build network` block: the NAT's name, prefix and host IP as read back from
the host (or the NAT a build would make), a NAT prefix that overlaps the build
link (BLOCKED, naming that NAT, or the default prefix when the host has none
yet), any other address on the NAT's vNIC (a warning naming the
`Remove-NetIPAddress` that removes it; two in the NAT prefix leave the host IP
ambiguous, which is BLOCKED), and for the durable strategy each slot switch, `free` or `in
use by` the VMs attached to it, or a missing-slots line naming `warden setup
--driver hyperv`.

Exits non-zero when a required capability is unmet, so it works as a CI or agent gate.
Replaces the old `warden hyperv doctor`.

## warden setup

```
warden setup --driver <name> [flags]
```

Idempotent host preparation. Hyper-V and Firecracker need it.

- `--driver hyperv` creates the durable network builds reuse: `--slots`
  Private build switches (default 4) named `<network>-0` to `<network>-<N-1>`
  (`--network`, default `warden-dev`), one per concurrent build, plus an
  Internal NAT switch, host IP and NAT rule for relay egress. Slot switches
  above `--slots` with nothing attached are removed, and the single
  `<network>` Private switch of earlier setups is retired when nothing is
  attached to it; either refuses, naming the VMs, while one is in use. Re-run
  it after upgrading from a warden whose setup made that single switch:
  durable builds refuse to start until the slots exist. Run once from an
  elevated terminal; afterwards builds run without elevation.
- `--driver firecracker` (as root, with `sudo`) creates `--slots` network slots
  owned by the user who ran sudo: per slot a bridge and three TAPs with their
  sysctls, plus the `warden_fc` bridge table and the `WARDEN-FC` iptables and
  ip6tables chains, and `net.ipv4.ip_forward=1` last. It refuses, printing the
  fix, when the filter FORWARD policy is ACCEPT, firewalld is running, or an
  nftables forward chain drops by default, and while any slot is in use (a VM
  has its TAPs open, or a build holds it), since the reload would leave that
  slot unfiltered. The slots are runtime state: run it
  after every boot (the [drivers guide](drivers.md#firecracker-driver) has a
  systemd unit). Afterwards builds need only `/dev/kvm`.

Other drivers print that no host setup is needed. Replaces the old
`warden hyperv setup`.

**Flags:**
- `--network <name>` — Durable network name, the prefix of its build switches `<name>-0` to `<name>-<N-1>` and of its NAT `<name>-nat` (default: `[vm]` network, then `warden-dev`)
- `--nat-prefix <cidr>` — NAT CIDR for relay egress (default 192.168.240.0/20).
  Every build reads the NAT back from the host and gives its relay its own
  address in this prefix, so a non-default value takes effect on every build.
  Refused, before anything changes, when it overlaps the build link (`[vm]
  build_link`, or the default `100.127.255.252/30`); setup has no
  `--build-link` of its own, and a build refuses an overlapping NAT too.
  Durable builds use only the NAT `<network>-nat`, so setup refuses, naming it
  and how to remove it, when another NAT already owns this prefix, and when
  `<network>-nat` exists on another prefix
- `--host-ip <ip>` — Host gateway IP on the NAT switch (default 192.168.240.1),
  the relay's upstream gateway on every build. When setup makes the NAT, the
  host IP is the only IPv4 address left on its vNIC, `vEthernet
  (<network>-nat)`: an address an earlier setup left there (on the old
  prefix, after that NAT was removed) is removed, and no other interface is
  touched
- `--slots <n>` — hyperv: build switches to create, 1 to 64 (default 4), the
  number of durable builds that can run at once; firecracker: network slots
  to create, 1 to 256 (default 16). Refused for other drivers

## warden hyperv service

```
warden hyperv service install|uninstall|run
```

Manage the privileged Hyper-V helper service (a LocalSystem Windows service
that performs only host network standup on behalf of non-elevated builds, over
a named pipe ACL'd to Administrators and Hyper-V Administrators). With it
installed, `warden build --driver hyperv` runs entirely without elevation and
stands up a fresh isolated network per build. VM lifecycle always runs
in-process; only network standup is delegated.

A service installed by a warden that speaks another protocol version is
stale: `warden doctor --driver hyperv` reports it, and a non-elevated build
is refused, naming the reinstall (`uninstall`, then `install`, elevated). A
stale service is never treated as absent, so the build does not fall back
to the durable network.

A service that is installed but not running (stopped, say) is reported as
such, not as not installed, with the fix: from an elevated shell, `Start-Service
WardenHyperV` (or `sc start WardenHyperV`); one that runs but whose pipe does
not answer, `Restart-Service WardenHyperV`. Doctor's blocked remedies then name
starting it rather than installing it. When the service manager will not give
this process the service's state, a failed read is never reported as not
installed: doctor says the state could not be read and why (`installed, but
its state could not be read` when access is denied, since the service manager
found the name), and names checking it from an elevated shell with
`Get-Service WardenHyperV`, starting it if it is stopped.

- `install` — Install and start the service (run once, elevated)
- `uninstall` — Stop and remove the service (elevated)
- `run` — Run the helper (invoked by the SCM; or in a console to smoke-test)

## warden clean

```
warden clean [--cache]
```

Remove orphaned containers, networks and per-build images left by interrupted
or crashed builds. Only removes resources not associated with a currently
running warden process, and nothing a later build would reuse: prepared and
downloaded VM images, the Hyper-V relay boot disk and the support images stay.

With `--driver firecracker` it needs no container runtime: it kills VMMs
recorded on a network slot that no build holds (left by a warden that was
killed, or by `--keep-vms`), and removes the work dirs and per-build rootfs
copies of builds that are no longer running and the partial raw images
(`*.raw.tmp`) of image conversions that were killed.

With `--driver hyperv` it needs no container runtime either: for each build
that is no longer running (left by a warden that was killed, or by
`--keep-vms`), it removes its `warden-build-<id>` and `warden-relay-<id>` VMs,
then its ephemeral `warden-<id>` switch, then its `warden-hyperv-<id>-*` work
dir (in `[build] tmpdir` when set, else the OS temp dir). A running build holds a lock
that keeps clean away from it. The durable switches and NAT from `warden
setup`, and prepare VMs (the next `warden image prepare` reaps those), stay.
Run it as a Hyper-V Administrator.

| Flag | Description |
|------|-------------|
| `--cache` | Also remove warden's whole cache directory. The next VM build downloads and prepares its images again. |

## Global Flags

| Flag | Description |
|------|-------------|
| `--driver <name>` | Orchestration driver (container, qemu, vz, hyperv, firecracker) |
| `--runtime <name>` | Container runtime (finch, docker, podman) |
| `--color <mode>` | Color output (auto, always, never) |
| `-v, --verbose` | Verbose output |
| `--version` | Print version |
| `-h, --help` | Help |
