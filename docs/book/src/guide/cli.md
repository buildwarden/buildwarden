# CLI Reference

## warden build

```
warden build [path] [flags]
```

Run an audited build with full network recording.

**Arguments:**
- `path` — Directory containing a Dockerfile, or path to a specific Dockerfile. Defaults to current directory.

**Flags:**
- `-o, --output <dir>` — Output directory (default: `warden-output`)
- `--driver <name>` — Build driver: container (default), qemu, vz
- `--image <path>` — Base disk image for VM drivers (QCOW2 for qemu, IPSW path for vz)
- `--script <path>` — Build script to run (VM drivers; alternative to Dockerfile translation)
- `--timeout <duration>` — Maximum build duration (e.g. "5m", "1h")
- `--capture <mode>` — Capture payloads: none, headers, bodies, all
- `--no-compress` — Disable zstd compression

**Example:**
```sh
# Container driver (default)
warden build ./my-project -o ./audit-results

# QEMU driver with a cloud image
warden build --driver qemu --image ubuntu-24.04.qcow2 ./my-project

# VZ driver with macOS IPSW
warden build --driver vz --image ~/Images/macOS-15.ipsw --script build.sh
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

**Example:**
```sh
warden inspect warden-output --json | jq '.summary'
```

## warden shell

```
warden shell [path] [flags]
```

Open an interactive shell in the audited build environment. Useful for debugging network behavior or exploring what a build fetches.

**Flags:** Same as `build`.

**Example:**
```sh
warden shell ./my-project
# Now inside the isolated environment — try curl, apt-get, etc.
# All traffic is recorded to the ledger.
```

## warden image

Manage VM images for the VZ driver.

### warden image list

```
warden image list
```

List prepared VM images in the image cache directory.

**Example:**
```sh
warden image list
# macOS-15.0   prepared   12.1 GB   2025-03-14
# macOS-15.4   prepared    6.8 GB   2025-05-20
```

### warden image prepare

```
warden image prepare [flags]
```

Update warden-io and the LaunchDaemon on a prepared macOS image. Requires sudo for disk image mounting.

**Example:**
```sh
sudo warden image prepare
# Mounting macOS-15.4...
# Updating warden-io binary...
# Updating LaunchDaemon plist...
# Done.
```

## warden clean

```
warden clean
```

Remove orphaned containers, networks, images, cached VM binaries, and stale build artifacts from interrupted or crashed builds. Only removes resources not associated with a currently running warden process.

**Example:**
```sh
warden clean
# Removing container: warden-build-deadbeef
# Removing container: warden-relay-deadbeef
# Removing network: warden-deadbeef
# Removing cached binary: .cache/warden/bin/relay-linux-amd64 (stale)
# Removed 4 orphaned resource(s).
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--driver <name>` | Build driver (container, qemu, vz) |
| `--runtime <name>` | Container runtime (finch, docker, podman) |
| `--color <mode>` | Color output (auto, always, never) |
| `-v, --verbose` | Show container runtime commands |
| `--version` | Print version |
| `-h, --help` | Help |
