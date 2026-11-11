# Installation

## Prerequisites

Prerequisites vary by driver:

### Container driver (any platform)

A container runtime is required. Any of the following will work:

- [Finch](https://github.com/runfinch/finch) (preferred on macOS)
- [Docker](https://docs.docker.com/get-docker/)
- [Podman](https://podman.io/)

### QEMU driver

Requires `qemu-system-{arch}` and `qemu-img`:

```sh
# macOS
brew install qemu

# Linux (Debian/Ubuntu)
apt install qemu-system
```

### Firecracker driver

Requires an x86_64 Linux host with KVM (`/dev/kvm`), your user in the `kvm`
group, and the `firecracker` binary (v1.17.0 or later) on `PATH`:

```sh
sudo usermod -aG kvm "$USER"     # then log in again
v=v1.17.0
curl -fsSLO https://github.com/firecracker-microvm/firecracker/releases/download/$v/firecracker-$v-x86_64.tgz
curl -fsSLO https://github.com/firecracker-microvm/firecracker/releases/download/$v/firecracker-$v-x86_64.tgz.sha256.txt
sha256sum -c firecracker-$v-x86_64.tgz.sha256.txt
tar -xzf firecracker-$v-x86_64.tgz
sudo install -m 0755 release-$v-x86_64/firecracker-$v-x86_64 /usr/local/bin/firecracker

# Host networking and disk-image tools (Debian/Ubuntu)
sudo apt install iproute2 nftables iptables qemu-utils genisoimage
```

### Hyper-V driver

Requires Windows with Hyper-V and its management stack enabled, and
membership in Hyper-V Administrators (or an elevated terminal). Building the
relay boot disk from a source tree also needs Git Bash and WSL.

### VZ driver

Requires macOS on Apple Silicon. No extra installation is needed — Virtualization.framework is built into macOS.

## One-liner Install

```sh
curl -sSfL https://raw.githubusercontent.com/buildwarden/buildwarden/main/install.sh | sh
```

This detects your OS and architecture, downloads the latest release, and installs to `/usr/local/bin/`. The released binary is ready for the container driver out of the box.

## From Releases

Download the appropriate binary from [GitHub Releases](https://github.com/buildwarden/buildwarden/releases/latest):

```sh
# macOS (Apple Silicon)
tar xzf buildwarden_*_darwin_arm64.tar.gz
mv warden /usr/local/bin/

# Linux (x86_64)
tar xzf buildwarden_*_linux_amd64.tar.gz
mv warden /usr/local/bin/
```

## From Source

```sh
git clone https://github.com/buildwarden/buildwarden.git
cd buildwarden
make build
```

Requires Go 1.26+ (managed via [mise](https://mise.jdx.dev/) if present).

The build produces the following binaries in `dist/`:

| Binary | Description |
|--------|-------------|
| `dist/warden` | Host binary (codesigned on macOS for VZ entitlement) |
| `dist/warden-relay-linux-arm64` | Relay binary for VM/container use |
| `dist/warden-io-linux-arm64` | Agent binary for Linux build environments |
| `dist/warden-io-darwin-arm64` | Agent binary for macOS VZ builds |

## First-time VZ Setup

After building from source on macOS Apple Silicon:

```sh
make build                                # Builds + codesigns with virtualization entitlement
sudo warden image prepare --driver vz     # Downloads macOS IPSW, restores VM, installs warden-io
```

The first `image prepare` downloads the latest supported macOS restore image
(about 25 GB), creates a VM disk, boots it, and installs the warden-io agent;
it runs under `sudo` because it writes root-owned files into the image. The
image it prepares becomes the *active* image, which builds use.

Later, the same command refreshes the active image's warden boot agent in
place, with no download, and warns when Apple has published a newer macOS.
Moving to that macOS is a separate step you choose:

```sh
sudo warden image prepare --driver vz            # refresh the active image in place
sudo warden image prepare --driver vz --update   # restore the latest supported macOS and make it active
warden image list                                # every prepared image, the active one first
```

You can also prepare from a `.ipsw` file you already have; it is read where it
is and not copied. The [drivers guide](../guide/drivers.md#first-time-setup)
covers sources, disk use and what to do when a build refuses an out-of-date
image, and the [CLI reference](../guide/cli.md#warden-image-prepare) lists
every option.

## First-time QEMU Setup

```sh
brew install qemu                           # macOS (or apt install qemu-system on Linux)
tools/relay-vm/build-initramfs.sh           # Build relay VM kernel + initramfs
tools/build-vm/build-initramfs.sh           # Build test VM initramfs (includes warden-io)
```

## First-time Firecracker Setup

```sh
make build
tools/firecracker-vm/build-kernel.sh                       # pinned guest kernel
ARCH=x86_64 NO_MODULES=1 tools/relay-vm/build-initramfs.sh # x86_64 relay initramfs
tools/firecracker-vm/build-initramfs.sh                    # direct-boot initramfs
sudo dist/warden setup --driver firecracker                # network slots (each boot)
dist/warden doctor --driver firecracker
```

The kernel build needs `make`, `gcc`, `flex`, `bison`, `bc`, `libelf-dev` and
`libssl-dev`. The slots do not survive a reboot; the
[drivers guide](../guide/drivers.md#firecracker-driver) shows a systemd unit
that re-creates them at boot. Setup refuses, with the fix, when the host's
`FORWARD` policy is `ACCEPT` or firewalld is running, and while any slot is in
use by a build or by VMs a `--keep-vms` build left running.

Optionally cache the default disk image before the first build, so that
build does not pay for the download and conversion:

```sh
dist/warden image prepare --driver firecracker   # ubuntu:24.04
```

The relay needs a DNS server that is not on the host itself. A host whose only
resolver is local (a resolver-less systemd-resolved stub, a local dnsmasq, or
WSL's `10.255.255.254`) gets an error that says how to fix it: give the host
an off-host resolver, or name one in `[vm] dns`; see
[Upstream DNS](../guide/drivers.md#upstream-dns).

WSL2 is experimental: it needs nested virtualization and a custom WSL kernel
built with `NF_TABLES_BRIDGE`, which the stock kernel leaves out, and WSL
loses the slots and its iptables state whenever its VM shuts down, so re-run
setup after each restart.

## First-time Hyper-V Setup

From a source tree, in Git Bash, then an elevated terminal for `setup`:

```sh
tools/relay-vm/hyperv/refresh-cache.sh   # relay boot disk (needs WSL)
warden setup --driver hyperv             # elevated, once: NAT and 4 build switches
warden image prepare --driver hyperv     # Windows Server base image (about 11 GB)
warden doctor --driver hyperv
```

`warden setup --driver hyperv` makes the durable network that builds run on
without elevation: an Internal NAT switch with its host IP and NAT rule, and
one Private build switch per build that can run at once, `warden-dev-0` to
`warden-dev-3` by default. Pass `--slots N` (1 to 64) for more or fewer
concurrent builds; re-running setup with a smaller number removes the
switches above it that nothing is attached to. When the default NAT prefix, `192.168.240.0/20`, is
already used on your network, pick another with `--nat-prefix` and
`--host-ip`; every build then gives its relay an address in that prefix (see
[Corporate Networks](../guide/corporate-networks.md#address-collisions-with-the-hyper-v-nat)).

Each build's relay resolves names through the IPv4 DNS servers of the host's
default-route interface, or only through `[vm] dns` when you set it. A host
whose only resolver is on the host itself is refused before any VM boots,
naming the interface and both fixes.

`warden doctor --driver hyperv` ends with a `Build network` block: the NAT it
read back (`warden-dev-nat (192.168.240.0/20, host 192.168.240.1)`), the
relay's resolvers and where they came from (`10.9.0.53 (from interface
Ethernet)`), and each build switch `free` or `in use by` the VMs on it. A
`BLOCKED` line there names its fix.

## Upgrading an Existing VM Driver Install

The VM drivers' relay-to-build link is now configurable (`[vm] build_link`),
and the launchers that read it changed. If you set up a VM driver before this
change, replace its launcher asset once: the Hyper-V relay boot VHDX
(`tools/relay-vm/hyperv/refresh-cache.sh`), the vz prepared image
(`sudo warden image prepare --driver vz`, in place), or the qemu and
Firecracker direct-boot initramfs (their `build-initramfs.sh`). Until you do,
a build refuses to start and names the command, and `warden doctor` reports
the asset's link-channel stamp as `missing` or `stale`. The full table is in
[Upgrading to the configurable build link](../guide/drivers.md#upgrading-to-the-configurable-build-link).

Hyper-V's per-build relay channel changed the relay boot disk and the durable
network once more. On a host set up before it, run two commands once:
`tools/relay-vm/hyperv/refresh-cache.sh`, since a build refuses a relay boot
disk from before the change and names that command; and, elevated,
`warden setup --driver hyperv`, which makes the per-build switches and retires
the single `warden-dev` build switch of earlier setups. Until you do, durable
(non-elevated) builds refuse to start and name `warden setup`; elevated and
service builds make their own switch and need only the relay disk. The qemu
and Firecracker relay initramfs need no rebuild.

## Verify Installation

```sh
warden --version
warden --help
```

BuildWarden auto-detects your container runtime for the container driver (trying finch, docker, podman in order). Override with `--runtime` or the `WARDEN_CTR_CLI` environment variable.
