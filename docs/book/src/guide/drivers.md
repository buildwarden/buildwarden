# Build Drivers

BuildWarden supports four build drivers, each providing the same hermetic build
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

On first use, prepare the macOS build image. This downloads an IPSW from Apple,
restores a macOS VM image, and installs `warden-io` and the LaunchDaemon into
it:

```sh
warden image prepare --driver vz
```

Re-running `warden image prepare --driver vz` on an already-restored image
refreshes its warden boot agent instead of restoring again. Subsequent builds
clone the prepared image (copy-on-write) for each build, so startup is fast.

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

---

## Hyper-V Driver

`--driver hyperv` (Windows only)

The Hyper-V driver runs builds inside a Hyper-V virtual machine. Like QEMU it
uses a two-VM topology: an Alpine relay VM provides the network bridge, and a
build VM (Windows or Linux) connects only through it. The relay VM reaches
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

- The relay VM boots from a UKI boot disk with `/sbin/relay` baked in
  (`tools/relay-vm/hyperv/build.sh`)
- It is reachable on a static host-only NAT IP (`192.168.240.2:8301`) for the
  claim; the build VM cannot reach the claim port
- Network isolation is topological, enforced by the switch layout

### Examples

```sh
warden build --driver hyperv --guest-os windows ./win-project
```

`--image <disk>` overrides the prepared base with a disk that already carries
the `warden-run` startup task; a stock, unprepared image would wait at OOBE.

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
build ran in a container, a QEMU VM, or a macOS VZ VM.

### Choosing a Driver

| | Container | QEMU | VZ | Hyper-V |
|---|---|---|---|---|
| **Platforms** | macOS, Linux, Windows | macOS, Linux, Windows | macOS (Apple Silicon) | Windows |
| **Isolation** | Network namespace + iptables | Topological (separate VM) | Topological (separate VM) | Topological (separate VM) |
| **Performance** | Native speed | Near-native (HVF/KVM) | Near-native (Hypervisor.framework) | Near-native (Hyper-V) |
| **Build OS** | Linux (container image) | Linux or Windows | macOS | Windows or Linux |
| **Setup effort** | Minimal | Moderate (relay VM assets) | Moderate (image prepare) | Moderate (host setup + image prepare) |
| **Best for** | Linux builds, CI/CD | Linux/Windows builds needing stronger isolation | macOS-native builds (Xcode, Swift) | Windows-native builds |
