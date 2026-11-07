# Build Drivers

BuildWarden supports five build drivers, each providing the same hermetic build
guarantee with different isolation technologies. The driver determines what
runs your build code and how network isolation is enforced.

## Driver Selection

By default, BuildWarden uses the container driver if a container runtime is
detected. Override with the `--driver` flag or a config file entry.

**Flag:**
```sh
warden build --driver qemu .
warden build --driver vz --script ./build.sh .
warden build --driver container .
```

**Config file** (`warden.toml`):
```toml
[runtime]
driver = "qemu"
```

---

## Container Driver

`--driver container` (default)

The container driver runs builds in an unprivileged OCI container on an
isolated Docker network. The relay runs as a sidecar container, and iptables
rules are applied by an init-container that shares the build container's
network namespace.

### Requirements

- Docker, Finch, or Podman
- Works on: macOS, Linux, Windows — any platform with a container runtime

No additional setup is needed beyond installing a container runtime.

Heavy builds (PyTorch, for example) need a larger runtime VM on macOS. For Finch,
in `~/.finch/finch.yaml`:

```yaml
cpus: 6
memory: 16GiB
datadisk: 150GiB
```

### How It Works

- Uses your Dockerfile/Containerfile as the build specification
- The relay container intercepts all network traffic (MITM proxy + DNS)
- The build container has no `CAP_NET_ADMIN` — it cannot modify its own
  network rules
- Network isolation is enforced via iptables in a one-shot init container

### Examples

Build the current directory (auto-detects Dockerfile):
```sh
warden build .
```

Specify a project path:
```sh
warden build --driver container ./my-project
```

Build with a specific Containerfile:
```sh
warden build ./examples/Dockerfile.pip
```

---

## QEMU Driver

`--driver qemu`

The QEMU driver runs builds inside a full virtual machine using QEMU's
system emulation. It uses a two-VM topology: a relay VM (Alpine Linux)
provides the network bridge, and a build VM (your target OS) connects only
through the relay. Network isolation is topological — the build VM's sole
network path passes through the relay.

### Requirements

- `qemu-system-aarch64` and `qemu-img`
- Works on arm64 hosts: macOS (HVF acceleration), Linux (KVM), or any host
  with TCG emulation (slow). x86_64 guests are not supported yet: the VMs use
  the arm64 `virt` machine type.

### Installation

macOS:
```sh
brew install qemu
```

Linux (Debian/Ubuntu):
```sh
apt install qemu-system-arm qemu-utils
```

Linux (Fedora/RHEL):
```sh
dnf install qemu-system-aarch64 qemu-img
```

### Setup

Build the relay VM assets (kernel + initramfs):
```sh
tools/relay-vm/build-initramfs.sh
```

Build the build VM assets (for direct-boot mode):
```sh
tools/build-vm/build-initramfs.sh
```

### Cloud Images

The QEMU driver uses QCOW2 cloud images as the build environment. Supported
FROM image names that auto-resolve to cloud images:

| FROM value | Image |
|---|---|
| `alpine` | Alpine latest |
| `alpine:3.21` | Alpine 3.21 |
| `ubuntu` | Ubuntu latest LTS |
| `ubuntu:24.04` | Ubuntu 24.04 |
| `debian` | Debian stable |
| `debian:bookworm` | Debian Bookworm |

Images are cached in `~/.cache/warden/images/` after first download.

### Examples

Build with an explicit image path:
```sh
warden build --driver qemu --image ~/.cache/warden/images/alpine.qcow2 .
```

Build using a Dockerfile with FROM auto-resolution:
```sh
warden build --driver qemu examples/Dockerfile.apk
```

The Dockerfile's `FROM alpine:3.21` line resolves to the cached Alpine QCOW2
image automatically.

### Windows guests

```sh
warden image prepare --driver qemu --os windows --source Win11_Arm64.iso
```

Windows images are made in two stages. The install stage runs an unattended
install from the ISO with no network and ends in `sysprep /generalize`, leaving
a generalized base like Microsoft's evaluation VHDs; it is redone only with
`--force` or a new ISO. The prepare stage then boots an overlay of that base
once with no network through OOBE and the same tuning as Hyper-V, registers
the `warden-run` startup task, and powers off; builds boot a throwaway child of
that layer as SYSTEM. With no `--source`, `image prepare` only re-prepares,
and builds start that in the background once the layer is
`[vm] image_refresh_after` old (see the Hyper-V section for the evaluation
grace rules, which are the same).

---

## VZ Driver

`--driver vz` (macOS Apple Silicon only)

The VZ driver runs builds inside a macOS virtual machine using Apple's
Virtualization.framework. The relay runs as a host process using gvisor
netstack, reading Ethernet frames from a socketpair. The build VM boots
macOS with a single network interface that connects exclusively to the
relay — no vmnet entitlement is needed.

### Requirements

- macOS on Apple Silicon (arm64)
- The `warden` binary must be codesigned with the
  `com.apple.security.virtualization` entitlement
- `make build` handles codesigning automatically if you have a Developer ID
  certificate

### First-Time Setup

On first use, prepare the macOS build image. This downloads the latest
supported IPSW from Apple, restores a macOS VM image, and installs `warden-io`
and the LaunchDaemon into it. It writes root-owned files into the image, so it
runs under `sudo`:

```sh
sudo warden image prepare --driver vz
```

Builds clone the prepared image (copy-on-write) for each build, so startup is
fast. The images live in `~/.cache/warden/images`, one `macos-<key>`
directory per IPSW, and `warden image list` lists them all.

**Which image builds use.** The image you prepared last is the *active*
image, and a build without `--image` uses it; `--image <dir>/disk.img` picks
another. A cache prepared before the active image was recorded uses the image
whose preparation finished last.

**Refreshing.** Re-running `sudo warden image prepare --driver vz` with no
source refreshes the active image's warden boot agent in place: no IPSW
download and no restore. When Apple has since published a newer macOS, it
ends with a warning naming both versions and the command to move to it:

```
warning: the active macOS image is macOS 27.0.1 (26A100); macOS 27.2 (26B123) is available. To update (about 25 GB, 15 minutes): sudo warden image prepare --driver vz --update
```

When the newer image is already prepared, the warning says so instead ("is
already prepared. To use it (no download)"). The check is best effort: offline,
the refresh completes and prints nothing.

**Moving to a newer macOS.** `sudo warden image prepare --driver vz --update`
restores the latest supported IPSW, or refreshes its image in place when it is
already prepared, and makes it active. A newer macOS is a different build OS,
so a refresh never moves to one by itself.

**Other sources.** A source names what to prepare, and the result becomes the
active image:

```sh
sudo warden image prepare --driver vz ~/.cache/warden/images/macos-90a1b1f94f64b00a   # refresh that image in place
sudo warden image prepare --driver vz ~/Downloads/UniversalMac_27.2_26B123_Restore.ipsw  # restore from a local IPSW
sudo warden image prepare --driver vz https://updates.cdn-apple.com/.../UniversalMac_27.2_26B123_Restore.ipsw
```

A local `.ipsw` file is read where it is: it is not copied into the cache and
never deleted. An IPSW whose image is already prepared is refreshed in place,
not restored again.

**Disk use.** Each image is about 25 GB on disk and older images are kept
(they are your rollback); delete a `macos-<key>` directory you no longer
want. An IPSW downloaded into the cache (`ipsw-<key>.ipsw`, about 25 GB) is
deleted once its image is prepared, since a later prepare of the same IPSW
refreshes the image in place. Restoring that image from scratch (after
deleting its directory) downloads the IPSW again.

### Examples

Run a build script inside a macOS VM:
```sh
warden build --driver vz --script ./build.sh .
```

### Network Architecture

- Uses `VZFileHandleNetworkDeviceAttachment` (no vmnet entitlement required)
- The host relay process creates a socketpair and attaches one end to the VM
- gvisor netstack processes raw Ethernet frames on the host side
- The build VM sees a single network interface whose only path is the relay
- The image's `warden-boot` takes its address by DHCP from the host relay,
  which answers from the build link in its claim, so the image bakes no
  address

---

## Hyper-V Driver

`--driver hyperv` (Windows only)

The Hyper-V driver runs builds inside a Hyper-V virtual machine. Like QEMU it
uses a two-VM topology: an Alpine relay VM provides the network bridge, and a
Windows build VM connects only through it (Linux build guests are not yet
supported on Hyper-V; use the QEMU driver). The relay VM reaches
upstream through an Internal NAT switch; the build VM sits on a Private build
switch with no other path.

### Requirements

- Windows with Hyper-V and its management stack enabled
- Membership in Hyper-V Administrators (or an elevated terminal)

### Setup

Create the durable network builds reuse (one time, from an elevated terminal):

```sh
warden setup --driver hyperv
```

Prepare a Windows Server base image. By default warden downloads Microsoft's
current Windows Server 2025 evaluation VHDX (about 11 GB) from the Evaluation
Center link; `--source` or `[vm] windows_image` names another URL, or a file
you already have (registered in place, not copied):

```sh
warden image prepare --driver hyperv
warden image prepare --driver hyperv --source D:\images\server2025.vhdx
```

Nothing is pinned in advance: the download comes over HTTPS (the default link
must resolve to a Microsoft host), and the resolved URL, ETag and the sha256
computed while downloading are recorded in the image manifest and in every
build's ledger. `--sha256` adds an expected-digest check. When Microsoft
publishes new media behind the link, the next refresh downloads it beside the
current base and prepares from it; `[vm] windows_image_update = "reuse"`
keeps the current base instead. A base you supplied as a local file is never
replaced automatically.

Prepare boots the image once with no network adapter. OOBE runs, a tuning
script turns off background work builds never need (Windows Update,
telemetry, maintenance tasks, automatic activation, Defender), registers the
`warden-run` startup task, and powers off. The result is a read-only
differencing layer over Microsoft's image, which is never modified. Every
build boots a throwaway child of that layer and runs as SYSTEM, with no OOBE.
Prepare takes about three minutes; the ledger records the chain (the
evaluation image, then the prepared layer with the hashes of its inputs).

The evaluation edition is never activated (prepare has no network), so a
prepared base has the 10-day initial grace period, counted in wall time from
the prepare. Builds keep it fresh: once the base is 7 days old, the next build
starts a background re-prepare (`warden image prepare --driver hyperv` with no
source does the same by hand), and a build refuses to start with less than a
day of grace left. Both ages are set in `[vm]` (`image_refresh_after`,
`image_min_grace`). Only one prepare runs at a time; superseded layers are
deleted by a later prepare once no VM uses them.

Optionally install the privileged helper service so builds run without
elevation and stand up a fresh isolated network per build:

```sh
warden hyperv service install     # run once, elevated
```

### How It Works

- The relay VM boots from a UKI boot disk with `/sbin/relay` baked in. From a
  source tree, `tools/relay-vm/hyperv/refresh-cache.sh` (Git Bash, needs WSL)
  builds it into the cache below, and rebuilds it only when the relay source
  has changed since; `--check` reports whether it is current
- It is reachable on a static host-only NAT IP (`192.168.240.2:8301`) for the
  claim; the build VM cannot reach the claim port
- Network isolation is topological, enforced by the switch layout

### Examples

```sh
warden build --driver hyperv --guest-os windows ./win-project
```

`--image <disk>` overrides the prepared base with a disk that already carries
the `warden-run` startup task; a stock, unprepared image would wait at OOBE.

### Where the image cache lives

Prepared images and the relay boot disk are cached in
`%LOCALAPPDATA%\warden\hyperv`, or in `WARDEN_HYPERV_CACHE_DIR` when set.
Hyper-V's own service opens these files, so they must be where it can see
them. When warden runs from a terminal started by a packaged (MSIX) app, such
as some desktop apps' built-in terminals, Windows can silently redirect new
files under `%LOCALAPPDATA%` into that app's private folder
(`%LOCALAPPDATA%\Packages\<app>\...`), which Hyper-V cannot see.
`warden doctor --driver hyperv` shows the cache as `REDIRECTED`, and
`warden image prepare` stops before any download with the fix: set
`WARDEN_HYPERV_CACHE_DIR` to a folder outside `AppData`, or run warden from an
ordinary terminal. The OS temp directory, where each build's work files go by
default, is not redirected.

---

## Firecracker Driver

`--driver firecracker` (x86_64 Linux with KVM)

The Firecracker driver runs each build in a Firecracker microVM, for Linux
hosts running many builds at once. It uses the QEMU driver's two-VM
topology: an Alpine relay VM, booted from the same relay initramfs and init,
and a Linux build VM whose only network interface is on a per-slot host
bridge whose only other port is the relay. Each VM is one `firecracker`
process with no API socket, no vsock, no MMDS and stdin `/dev/null`.

### Requirements

- An x86_64 Linux host with `/dev/kvm`, and your user in the `kvm` group
- Firecracker v1.17.0 or later on `PATH`
- `nft`, `iptables` and `ip6tables` (legacy or nft), and `ip` (iproute2)
- For disk-image builds: `qemu-img` (`qemu-utils`) and `genisoimage` or
  `mkisofs`
- An upstream DNS server that is not on the host itself (see below)

arm64 hosts are not supported yet.

### Boot assets

Both VMs boot one pinned guest kernel (Linux 6.1 LTS with every feature
built in), and the driver needs two initramfs images. Build them once from
the source tree:

```sh
tools/firecracker-vm/build-kernel.sh                     # output/vmlinux
ARCH=x86_64 NO_MODULES=1 tools/relay-vm/build-initramfs.sh
tools/firecracker-vm/build-initramfs.sh                  # direct-boot initramfs
```

warden finds them beside its binary (`firecracker-vmlinux`,
`firecracker-relay-initramfs.cpio.gz`, `firecracker-build-initramfs.cpio.gz`)
or in those scripts' output directories.

### Setup

Builds run unprivileged. A root setup step, once per boot, creates the
network slots they take:

```sh
sudo warden setup --driver firecracker            # 16 slots
sudo warden setup --driver firecracker --slots 64 # up to 256
```

Each slot `i` is a bridge `wfc<i>b` with no address joining two TAPs (the
relay's build NIC `wfc<i>r` and the build VM's `wfc<i>g`), plus the relay's
upstream TAP `wfc<i>u` on a /30 in `100.64.88.0/22`. The TAPs belong to the
user who ran sudo. Setup also loads the `warden_fc` bridge table and the
`WARDEN-FC` iptables chains that isolate the slots (see
[Network Isolation](../concepts/network-isolation.md#firecracker-slots)),
and sets `net.ipv4.ip_forward=1` last. It is idempotent: existing
interfaces are verified, an upstream TAP that already has its address keeps
it, and warden's own table and chains are reloaded; no other rule is touched.

Because the reload briefly removes the slots' rules, setup refuses while any
slot is in use: a VM has one of its TAPs open (a running build, VMs left by
`--keep-vms`, or ones a killed warden left behind), or a build holds the
slot. Wait for the builds to finish, or stop leftover VMs with `warden clean
--driver firecracker` run as the build user, then re-run setup. While setup
runs it holds every slot's lock, so a build starting then finds them all in
use. It creates any lock file a build has not made yet, as the build user, in
`~/.cache/buildwarden/locks`. It uses `XDG_CACHE_HOME` instead only when sudo
keeps it along with your `HOME`; if your builds keep their cache elsewhere
and sudo drops it, setup holds the wrong locks, so start no build while it
runs.

Setup refuses, printing the fix, when the host's firewall would make it
unsafe or ineffective:

- the filter `FORWARD` policy is `ACCEPT` (enabling forwarding would route
  between your other interfaces): `iptables -P FORWARD DROP && ip6tables -P
  FORWARD DROP`, kept across reboots;
- firewalld is running: put the upstream TAPs in its trusted zone,
  `firewall-cmd --permanent --zone=trusted --add-interface=wfc+u`;
- an nftables forward chain drops by default: setup prints the `nft insert
  rule` lines that let `wfc*u` traffic through.

The slots are runtime state and vanish at reboot. Run setup at boot with a
systemd unit (`builder` is the user builds run as):

```ini
# /etc/systemd/system/warden-firecracker.service
[Unit]
Description=BuildWarden Firecracker network slots
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
Environment=SUDO_USER=builder
ExecStart=/usr/local/bin/warden setup --driver firecracker --slots 16

[Install]
WantedBy=multi-user.target
```

To remove the slots, delete the interfaces (`ip link del wfc<i>b`, and the
same for each TAP), `nft delete table bridge warden_fc`, and remove the
`WARDEN-FC-*` chains and the jumps into them. warden has no command for it.

`warden doctor --driver firecracker` checks all of the above; run it with
sudo to include the rules and the FORWARD policy, which only root can read.
Under sudo the other checks are of the user who ran sudo (their slots, kvm
group and leftover VMMs), and the reflink probe is skipped: run doctor
without sudo for it.

### Upstream DNS

The relay resolves names for the build through the host's resolvers: those
in `/etc/resolv.conf`, or systemd-resolved's upstream list when its
`127.0.0.53` stub fronts them. A loopback resolver or one on the host's own
address (a local dnsmasq, unbound or libvirt) is left out, since the slot
rules do not let the relay open connections to the host. So is one inside
the build link (`[vm] build_link`, default `100.127.255.252/30`), the relay's
link to the build VM: the relay would send its queries to the build VM. The
default is a range no VPC or LAN resolver uses, so an AWS VPC's own
resolver is used as is. With none left the build stops before booting and
says how to fix it: a resolver outside the link, or another `[vm]
build_link`. Under WSL, whose only
resolver is its DNS-tunnelling address `10.255.255.254`, set
`generateResolvConf=false` under `[network]` in `/etc/wsl.conf` and list the
Windows host's DNS servers in `/etc/resolv.conf`.

### Images

A build boots one of two ways:

- **Disk image.** With a `FROM` the qemu catalog resolves (`ubuntu:24.04`,
  `debian:bookworm`, `alpine:3.21`) or `--image <path>` (qcow2 or raw), the
  image is converted to raw once into `~/.cache/warden/firecracker/images`
  and copied per build into `~/.cache/warden/firecracker/work/<id>` with
  `cp --reflink=auto` (instant on btrfs or XFS, a full copy elsewhere).
  warden delivers `warden-io` on a cloud-init `CIDATA` seed on a read-only
  drive; the image's own kernel is not used. An image with a backing file or
  an external data file is refused.
- **Direct boot.** With no resolvable image, the build VM boots a small
  initramfs that holds only a launcher, with `warden-io` appended per build.

Prepare an image ahead of the first build:

```sh
warden image prepare --driver firecracker              # ubuntu:24.04
warden image prepare --driver firecracker ./custom.qcow2
```

### Examples

```sh
warden build --driver firecracker .                        # Dockerfile FROM ubuntu:24.04
warden build --driver firecracker --script build.sh ./src  # direct boot
warden build --driver firecracker --cpus 8 --memory 8192 .
```

`--keep-vms` leaves both VMs running and the slot held, and prints their
pids; `warden clean --driver firecracker` stops them. It also kills any VMM
left by a warden that was killed, and removes stale work dirs; the next
build on a slot kills such leftovers too.

---

## Upgrading to the configurable build link

The VM drivers' relay-to-build link moved from a fixed RFC 1918 `/30` to the
configurable `[vm] build_link` (default `100.127.255.252/30`; see
[Configuration](configuration.md#the-build-link-vm-build_link---build-link)).
The link now reaches every component per build, so a launcher built before
the change, which bakes `10.0.0.x`, is replaced once. Each such asset carries
a link-channel stamp; a build refuses an asset without a current one, naming
the command, and `warden doctor` reports each stamp's state. After this one
step, changing `[vm] build_link` needs nothing on any driver.

| Driver | Replace once | With |
|---|---|---|
| qemu (direct boot only) | the direct-boot initramfs | `tools/build-vm/build-initramfs.sh` |
| firecracker | the direct-boot launcher initramfs | `tools/firecracker-vm/build-initramfs.sh` |
| hyperv | the relay boot VHDX (it bakes the relay binary) | `tools/relay-vm/hyperv/refresh-cache.sh` |
| vz | the prepared macOS image's `warden-boot` | `sudo warden image prepare --driver vz` (refreshes the active image in place, no IPSW restore) |

The qemu and Firecracker relay initramfs need nothing: the relay addresses
its build interface from the claim and replaces an old init's address. Disk
image builds on qemu and Firecracker, and Windows guests, take the link from
their per-build seed and need nothing either.

On vz, `warden image prepare` refreshes only images in the cache
(`~/.cache/warden/images`), and with no source only the active image. A build
with `--image` naming another cache image whose `warden-boot` predates the
change is refused with the fix `sudo warden image prepare --driver vz <dir>`
(which refreshes that image and makes it active) or to omit `--image` and
build from the active image. A build with `--image` naming a disk outside the
cache is refused with the fix to omit `--image` and build from the cache's
prepared image.

---

## Common Patterns

Regardless of which driver you use, the build environment works the same way
from the perspective of your build script.

### warden-io Commands

Inside every build environment, `warden-io` is available for interacting with
the relay:

Fetch a context file into the build environment:
```sh
warden-io fetch myfile.tar.gz -o /tmp/myfile.tar.gz
```

Post a build artifact (recorded in the ledger):
```sh
warden-io post ./dist/package.whl
```

Post with a custom name:
```sh
warden-io post ./output/binary my-tool-v1.0
```

Establish TLS trust (install relay CA):
```sh
warden-io trust
```

### Inspecting Build Output

The ledger output is identical across all drivers:
```sh
warden inspect warden-output/
```

This verifies the Ed25519 signature chain and displays all recorded network
activity, context fetches, and artifact posts — regardless of whether the
build ran in a container, a QEMU or Firecracker VM, or a macOS VZ VM.

### Choosing a Driver

| | Container | QEMU | VZ | Hyper-V | Firecracker |
|---|---|---|---|---|---|
| **Platforms** | macOS, Linux, Windows | macOS, Linux, Windows | macOS (Apple Silicon) | Windows | Linux x86_64 with KVM |
| **Isolation** | Network namespace + iptables | Topological (separate VM) | Topological (separate VM) | Topological (separate VM) | Topological (microVM, per-slot bridge) |
| **Performance** | Native speed | Near-native (HVF/KVM) | Near-native (Hypervisor.framework) | Near-native (Hyper-V) | Near-native (KVM), fast boot |
| **Build OS** | Linux (container image) | Linux or Windows | macOS | Windows or Linux | Linux |
| **Setup effort** | Minimal | Moderate (relay VM assets) | Moderate (image prepare) | Moderate (host setup + image prepare) | Moderate (boot assets + root setup each boot) |
| **Best for** | Linux builds, CI/CD | Linux/Windows builds needing stronger isolation | macOS-native builds (Xcode, Swift) | Windows-native builds | Many concurrent Linux builds on one host |
