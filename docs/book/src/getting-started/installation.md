# Installation

## Prerequisites

Prerequisites vary by driver:

### Container driver (any platform)

A container runtime is required. Any of the following will work:

- [Finch](https://github.com/runfinch/finch) (preferred on macOS)
- [Docker](https://docs.docker.com/get-docker/)
- [Podman](https://podman.io/)

`warden` runs the runtime by its name (`finch`, `docker` or `podman`), which
it looks up on `PATH`. If yours is installed outside `PATH`, as it often is
for a CI job or a service, add its directory to `PATH`: the runtime setting
takes one of the three names, not a path.

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

Requires macOS on Apple Silicon. Virtualization.framework is built into macOS,
but the vz driver is only in a `warden` you build [from source](#from-source)
on the Mac: a released `warden` has none, and `--driver vz` says so (see the
[drivers guide](../guide/drivers.md#vz-driver)). The build also needs the
Xcode Command Line Tools (`xcode-select --install`).

## One-liner Install

```sh
curl -sSfL https://raw.githubusercontent.com/buildwarden/buildwarden/main/install.sh | sh
```

This detects your OS and architecture, downloads the latest release, and installs `warden`, the container driver `warden-driver-container` and `warden-io` to `/usr/local/bin/`. The container driver is a program of its own, which warden finds beside itself; with it, the release is ready for the container driver out of the box. It is built without cgo, so it has no vz driver: for vz, build `warden` from source on the Mac and follow the [first-time VZ setup](#first-time-vz-setup).

## From Releases

Download the appropriate binary from [GitHub Releases](https://github.com/buildwarden/buildwarden/releases/latest):

```sh
# macOS (Apple Silicon)
tar xzf buildwarden_*_darwin_arm64.tar.gz
mv warden warden-driver-container warden-io /usr/local/bin/

# Linux (x86_64)
tar xzf buildwarden_*_linux_amd64.tar.gz
mv warden warden-driver-container warden-io /usr/local/bin/
```

Keep `warden-driver-container` in the same directory as `warden`: warden
looks for its drivers on `PATH`, then beside itself ([Drivers](../guide/drivers.md#where-a-driver-comes-from)).

These are the same binaries the one-liner installs, so the macOS one has no vz
driver either.

## From Source

```sh
git clone https://github.com/buildwarden/buildwarden.git
cd buildwarden
make build
```

Requires Go 1.26+ (managed via [mise](https://mise.jdx.dev/) if present). On
a Mac it also needs the Xcode Command Line Tools: `make build` compiles the vz
driver with cgo, which a release does not.

The build produces the following binaries in `dist/`:

| Binary | Description |
|--------|-------------|
| `dist/warden` | Host binary (codesigned on macOS for VZ entitlement) |
| `dist/warden-driver-container` | The container driver, which warden runs from beside itself |
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
make build                                  # dist/warden and the guest binaries
tools/relay-vm/build-initramfs.sh           # Build relay VM kernel + initramfs
tools/build-vm/build-initramfs.sh           # Build test VM initramfs (includes warden-io)
dist/warden doctor --driver qemu
```

Run the VM drivers as `dist/warden` from this tree: `warden` finds the relay
VM assets and the source it builds guest binaries from only in the tree its
executable is in.

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
`BLOCKED` line there names its fix. An address other than the host IP on the
NAT's vNIC, `vEthernet (warden-dev-nat)`, is a warning naming the
`Remove-NetIPAddress` that clears it; setup leaves only the host IP there when
it makes the NAT.

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

<!-- TRANSITIONAL(runtime-former-values): this section is removed with the
     "A path, or another runtime" subsection of guide/configuration.md, in
     the change that prepares 1.0. -->
## Upgrading: the Container Runtime Setting

Earlier releases ran whatever program `--runtime`, `WARDEN_CTR_CLI` or
`[runtime] cli` named. The setting now takes `finch`, `docker` or `podman`
and nothing else, wherever it is set. If yours names one of the three or is
unset, nothing changes. Any other value is refused by the first command that
uses the container runtime (`build`, `shell` and `doctor` with the container
driver; `clean` with the container driver, `qemu` or `vz`), before anything
is run, and the error says where the value is set and what to change.

Two kinds of value have a replacement:

- A path to a supported runtime (`/opt/homebrew/bin/docker`), or its name in
  another form (`Docker`, `docker.exe`): write the name, `docker`, and add
  the runtime's directory to `PATH` if it is not there.
- A wrapper script that only set variables for a supported runtime
  (`DOCKER_HOST`, `DOCKER_CONTEXT`, `CONTAINER_HOST`): name the runtime and
  set those variables in the environment `warden` runs in.

`nerdctl`, and a wrapper that added `sudo` or arguments, have none.
[A path, or another runtime](../guide/configuration.md#a-path-or-another-runtime)
lists every value that stopped working and the message each one gets.

## Upgrading: Driver Binaries and Renamed Keys

The container driver is now a binary of its own, `warden-driver-container`,
which warden runs from beside itself. The one-liner and the release archives
install it; if you copy `warden` by hand, copy `warden-driver-container` into
the same directory. `warden driver list` shows which file each driver name
runs and where it was found, and a build records the same in its output's
[`driver.json`](../guide/output.md#driverjson).

A driver's settings moved into `[driver]` and `[driver.<name>]`. An existing
`warden.toml` keeps working: the old names are read until BuildWarden 1.0,
and each one prints a warning naming its new key:

```
[warden] warning: /src/repo/warden.toml: [runtime] cli is deprecated; use [driver.container] cli
```

Rename them when you next edit the file. `[runtime] cli = "podman"` becomes:

```toml
[driver.container]
cli = "podman"
```

[Renamed keys](../guide/configuration.md#renamed-keys) lists all seven. A file
that sets both names of one key is refused. A driver's keys are now
case-sensitive: `[Runtime] CLI` is refused with the spelling to write, see
[Keys are case-sensitive](../guide/configuration.md#keys-are-case-sensitive).
`[runtime] driver` is warden's own key and does not change.

## Verify Installation

```sh
warden --version
warden --help
warden driver list   # the drivers warden finds, and the file each one runs
```

BuildWarden auto-detects your container runtime for the container driver (trying finch, docker, podman in order). Override with `--runtime` or the `WARDEN_CTR_CLI` environment variable; see [The container runtime](../guide/configuration.md#the-container-runtime).
