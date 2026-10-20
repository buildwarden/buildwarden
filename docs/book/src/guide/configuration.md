# Configuration

BuildWarden looks for configuration in two places, merged in order:

1. `~/.config/warden/config.toml` — user-level defaults
2. `./warden.toml` — project-level overrides

CLI flags override everything.

## Full Reference

```toml
[runtime]
cli = "finch"              # Container runtime: finch, docker, podman
driver = ""                # Build driver: container, qemu, vz (default: auto)
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
cpus = 0                   # Build VM vCPUs (0 = per-driver default)
memory_mb = 0              # Build VM memory in MiB (0 = per-driver default)
network = ""               # hyperv: durable build switch name
image_refresh_after = "7d" # hyperv: re-prepare the base in the background at this age
image_min_grace = "1d"     # hyperv: refuse builds with less evaluation grace left

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
| `--driver` | Global | Orchestration driver (container, qemu, vz, hyperv) |
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
| `-q, --quiet` | build | Do not echo the guest build console |

## Runtime Autodetection

If no runtime is configured, BuildWarden probes in order: **finch** → docker → podman. The first one that responds to `<runtime> info` within 3 seconds wins.

## Driver Selection

When `driver` is empty (the default), the **container** driver is used. Use `--driver` or `WARDEN_DRIVER` to select a VM driver:

- `container` — Unprivileged container with iptables isolation (default)
- `qemu` — Two-VM topology, cross-platform, requires `qemu-system-*`
- `vz` — macOS/arm64 only, uses Virtualization.framework
- `hyperv` — Windows, two-VM topology on Hyper-V with a host-only NAT switch

## Relay Image

For release builds (`warden --version` shows a version tag), the relay is pulled as a pre-built container image. For development builds (`version = "dev"`), it's compiled from source.

Override with `relay_image`:
- `"dev"` — force build from source (requires Go toolchain)
- `"ghcr.io/buildwarden/relay:v0.1.0"` — pin a specific version
- `""` — automatic (pull for releases, build for dev)
