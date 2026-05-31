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
timeout = ""               # Build timeout (e.g. "10m")

[qemu]
accel = ""                 # QEMU accelerator: hvf, kvm, whpx, tcg (default: auto-detect)
binary = ""                # Override qemu-system binary path
cpus = 4                   # Build VM CPU count
memory = 4096              # Build VM memory (MB)

[vz]
cpus = 4                   # Build VM CPU count
memory = 8192              # Build VM memory (MB)
image_dir = ""             # Override image cache directory (default: ~/.cache/warden/images)

[output]
color = "auto"             # Color mode: auto, always, never
verbose = false            # Verbose output (shows container runtime commands)
```

## Environment Variables

| Variable | Overrides | Description |
|----------|-----------|-------------|
| `WARDEN_DRIVER` | `runtime.driver` | Override driver selection |
| `WARDEN_IMAGE` | — | Base image path for VM drivers |
| `WARDEN_CTR_CLI` | `runtime.cli` | Container runtime binary name |
| `NO_COLOR` | `output.color` | Disable colored output (any value) |
| `WARDEN_VERBOSE` | `output.verbose` | Enable verbose mode (any value) |

## CLI Flags

| Flag | Scope | Description |
|------|-------|-------------|
| `--driver` | Global | Build driver |
| `--runtime` | Global | Container runtime |
| `--color` | Global | Color mode |
| `-v, --verbose` | Global | Verbose output |
| `-o, --output` | build/shell | Output directory |
| `--image` | build | Base disk image for VM drivers |
| `--script` | build | Build script (VM drivers) |
| `--timeout` | build | Maximum build duration |
| `--capture` | build/shell | Payload capture mode |
| `--no-compress` | build/shell | Disable compression |

## Runtime Autodetection

If no runtime is configured, BuildWarden probes in order: **finch** → docker → podman. The first one that responds to `<runtime> info` within 3 seconds wins.

## Driver Selection

When `driver` is empty (the default), BuildWarden selects a driver automatically:
- If a Dockerfile is present, the **container** driver is used.
- If `--image` points to a `.qcow2` file, the **qemu** driver is selected.
- If `--image` points to an `.ipsw` file or the platform is macOS with Virtualization.framework available, the **vz** driver is selected.

Explicit `--driver` or `WARDEN_DRIVER` overrides autodetection.

## Relay Image

For release builds (`warden --version` shows a version tag), the relay is pulled as a pre-built container image. For development builds (`version = "dev"`), it's compiled from source.

Override with `relay_image`:
- `"dev"` — force build from source (requires Go toolchain)
- `"ghcr.io/buildwarden/relay:v0.1.0"` — pin a specific version
- `""` — automatic (pull for releases, build for dev)
