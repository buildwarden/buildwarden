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
make build                    # Builds + codesigns with virtualization entitlement
sudo warden image prepare     # Downloads macOS IPSW, restores VM, installs warden-io
```

The `image prepare` step downloads a macOS restore image, creates a VM disk, boots it, and installs the warden-io agent. This only needs to be done once (or when upgrading).

## First-time QEMU Setup

```sh
brew install qemu                           # macOS (or apt install qemu-system on Linux)
tools/relay-vm/build-initramfs.sh           # Build relay VM kernel + initramfs
tools/build-vm/build-initramfs.sh           # Build test VM initramfs (includes warden-io)
```

## Verify Installation

```sh
warden --version
warden --help
```

BuildWarden auto-detects your container runtime for the container driver (trying finch, docker, podman in order). Override with `--runtime` or the `WARDEN_CTR_CLI` environment variable.
