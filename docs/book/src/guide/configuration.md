# Configuration

BuildWarden looks for configuration in two places, merged in order:

1. `~/.config/warden/config.toml` — user-level defaults
2. `./warden.toml` — project-level overrides

CLI flags override everything.

## Full Reference

```toml
[runtime]
cli = "finch"              # Container runtime: finch, docker, podman
driver = ""                # Build driver: container, qemu, vz, hyperv, firecracker
                           #   (default: auto, the container driver)
relay_image = ""           # Relay image override (container driver)
                           #   "" = pull ghcr.io/buildwarden/relay:latest (release builds)
                           #        or build from source (dev builds)
                           #   "dev" = always build from source
                           #   "ghcr.io/buildwarden/relay:v0.2.0" = specific version

[build]
dockerfile = ""            # Override Dockerfile discovery
context = ""               # Override context directory
output_dir = "warden-output"  # Where build results are written
compress = true            # Compress ledger and payloads with zstd
capture = ""               # Payload capture mode:
                           #   "" or "none" = no capture
                           #   "headers" = save request/response headers
                           #   "bodies" = save request/response bodies
                           #   "all" = save both
tmpdir = ""                # Where each build's private work dir is created
                           #   ("" = the OS temp dir; --tmpdir wins)

[relay]
upstream_ca_certs = []     # Paths to additional CA cert bundles for upstream
system_ca_bundle = true    # Include host system CA bundle in relay trust store

[vm]                       # Defaults for the VM-driver flags; a flag always wins
cpus = 0                   # Build VM vCPUs (0 = per-driver default; vz, qemu,
                           #   hyperv and firecracker)
memory_mb = 0              # Build VM memory in MiB (0 = per-driver default)
network = ""               # hyperv: durable build switch name
image_refresh_after = "7d" # hyperv: re-prepare the base in the background at this age
image_min_grace = "1d"     # hyperv: refuse builds with less evaluation grace left
windows_image = ""         # hyperv: base image URL or local path (default: Microsoft's
                           # current Server 2025 evaluation download)
windows_image_update = "auto" # hyperv: "auto" picks up new media at refresh; "reuse" keeps it
build_link = "100.127.255.252/30" # advanced: the relay-to-build link, a /30 (every VM
                           #   driver; see Advanced options; --build-link wins)

[output]
color = "auto"             # Color mode: auto, always, never
verbose = false            # Verbose output (shows container runtime commands)
```

## Environment Variables

| Variable | Overrides | Description |
|----------|-----------|-------------|
| `WARDEN_DRIVER` | `runtime.driver` | Override driver selection |
| `WARDEN_IMAGE` | `runtime.relay_image` | Relay image override |
| `WARDEN_CTR_CLI` | `runtime.cli` | Container runtime binary name |
| `WARDEN_UPSTREAM_CA_CERTS` | `relay.upstream_ca_certs` | Extra upstream CA files/dirs (colon-separated) |
| `NO_COLOR` | `output.color` | Disable colored output (any value) |
| `WARDEN_VERBOSE` | `output.verbose` | Enable verbose mode (any value) |

## CLI Flags

| Flag | Scope | Description |
|------|-------|-------------|
| `--driver` | Global | Orchestration driver (container, qemu, vz, hyperv, firecracker) |
| `--runtime` | Global | Container runtime |
| `--color` | Global | Color mode |
| `-v, --verbose` | Global | Verbose output |
| `-o, --output` | build/shell | Output directory |
| `--capture` | build/shell | Payload capture mode |
| `--no-compress` | build/shell | Disable compression |
| `--timeout` | build/shell | Maximum build duration (all drivers) |
| `--image` | build/shell | Base disk image for the build VM (VM drivers) |
| `--script` | build/shell | Build script (VM drivers) |
| `--guest-os` | build/shell | Guest OS: linux (default) or windows (qemu, hyperv) |
| `--cpus` | build/shell | Build VM vCPUs (VM drivers) |
| `--memory` | build/shell | Build VM memory in MiB (VM drivers) |
| `--network` | build/shell | Durable switch to reuse (hyperv) |
| `--keep-vms` | build/shell | Leave VMs and work dir for inspection (VM drivers) |
| `--tmpdir` | build/shell | Directory for each build's private work dir (default: OS temp dir) |
| `--build-link` | build/shell/doctor | Advanced: the VM drivers' relay-to-build link, a /30 (see Advanced options) |
| `-q, --quiet` | build | Do not echo the guest build console |

## Advanced options

### The build link (`[vm] build_link`, `--build-link`)

Every VM driver joins the build VM to the relay over one private link, a
`/30`: the relay is its first host and the build VM its second. The default
is `100.127.255.252/30` (relay `100.127.255.253`, build VM
`100.127.255.254`), the last `/30` of RFC 6598 shared address space, which no
common LAN, VPC or resolver plan uses. The container driver has its own
per-build network and ignores a valid setting. A malformed `[vm] build_link`
is refused when the config file loads, by every command and with every
driver (container included), as any config error is.

Inside the relay the link is a connected route, so an upstream address inside
it (a resolver, a private mirror, a collector) is sent to the build VM
instead of upstream. Move the link only when your network uses the default
anyway:

```toml
[vm]
build_link = "100.126.255.252/30"
```

or, for one command, `--build-link 100.126.255.252/30` on `build`, `shell`
or `doctor`. Precedence: the flag, then `./warden.toml`, then
`~/.config/warden/config.toml`, then the default. `warden doctor` prints the
link and where it came from.

The value must be an IPv4 network address with prefix exactly `/30`
(`100.126.0.1/30` is refused with "did you mean 100.126.0.0/30"), outside
`0.0.0.0/8`, loopback, link-local, multicast and class E, and disjoint from
the ranges BuildWarden uses itself: the container driver's `100.64.87.0/24`,
Firecracker's upstream `100.64.88.0/22`, Hyper-V's default NAT
`192.168.240.0/20` and qemu's user-mode `10.0.2.0/24`. A key is checked when
the file loads and the flag when it is read, before any work. Every VM build
also refuses a link that holds an address of one of the host's interfaces;
a host network that merely contains the link (a `/10` CGNAT WAN, say) is
only a doctor warning.

The link reaches the relay in its claim and the build VM in its seed, its
kernel command line or the relay's DHCP lease, per build: changing it needs
no rebuild, refresh or re-prepare. The one-time upgrade to this scheme is in
[Drivers](drivers.md#upgrading-to-the-configurable-build-link).

## Runtime Autodetection

If no runtime is configured, BuildWarden probes in order: **finch** → docker → podman. The first one that responds to `<runtime> info` within 3 seconds wins.

## Driver Selection

When `driver` is empty (the default), the **container** driver is used. Use `--driver` or `WARDEN_DRIVER` to select a VM driver:

- `container` — Unprivileged container with iptables isolation (default)
- `qemu` — Two-VM topology, cross-platform, requires `qemu-system-*`
- `vz` — macOS/arm64 only, uses Virtualization.framework
- `hyperv` — Windows, two-VM topology on Hyper-V with a host-only NAT switch
- `firecracker` — x86_64 Linux with KVM, two Firecracker microVMs on a network
  slot made by `sudo warden setup --driver firecracker`; no config keys of its own

## Relay Image

For release builds (`warden --version` shows a version tag), the relay is pulled as a pre-built container image. For development builds (`version = "dev"`), it's compiled from source.

Override with `relay_image`:
- `"dev"` — force build from source (requires Go toolchain)
- `"ghcr.io/buildwarden/relay:v0.1.0"` — pin a specific version
- `""` — automatic (pull for releases, build for dev)
