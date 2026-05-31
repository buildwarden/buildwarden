# Build Drivers

BuildWarden supports three build drivers, each providing the same hermetic build
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

- `qemu-system-aarch64` (or `qemu-system-x86_64` for x86 hosts)
- `qemu-img`
- Works on: macOS (HVF acceleration), Linux (KVM), Windows (WHPX), any (TCG fallback)

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

1. **Image restore** — On first use, the VZ driver downloads an IPSW from
   Apple and restores a macOS VM image. This is a one-time operation.

2. **Image preparation** — Install `warden-io` and the LaunchDaemon into the
   macOS disk image:
   ```sh
   sudo warden image prepare
   ```
   This must run as root because it mounts and modifies the VM disk image.

3. **Subsequent builds** — The prepared image is cloned (copy-on-write) for
   each build, so startup is fast.

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

| | Container | QEMU | VZ |
|---|---|---|---|
| **Platforms** | macOS, Linux, Windows | macOS, Linux, Windows | macOS (Apple Silicon) |
| **Isolation** | Network namespace + iptables | Topological (separate VM) | Topological (separate VM) |
| **Performance** | Native speed | Near-native (HVF/KVM) | Near-native (Hypervisor.framework) |
| **Build OS** | Linux (container image) | Linux (cloud image) | macOS |
| **Setup effort** | Minimal | Moderate (build VM assets) | Moderate (image restore + prepare) |
| **Best for** | Linux builds, CI/CD | Linux builds needing stronger isolation | macOS-native builds (Xcode, Swift) |
