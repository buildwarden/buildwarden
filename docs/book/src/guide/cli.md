# CLI Reference

## warden build

```
warden build [path] [flags]
```

Run a containerized build with full network auditing.

**Arguments:**
- `path` — Directory containing a Dockerfile, or path to a specific Dockerfile. Defaults to current directory.

**Flags:**
- `-o, --output <dir>` — Output directory (default: `warden-output`)
- `--capture <mode>` — Capture payloads: none, headers, bodies, all
- `--no-compress` — Disable zstd compression

**Example:**
```sh
warden build ./my-project -o ./audit-results
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
# Now inside the isolated container — try curl, apt-get, etc.
# All traffic is recorded to the ledger.
```

## warden clean

```
warden clean
```

Remove orphaned containers, networks, and images from interrupted or crashed builds. Only removes resources not associated with a currently running warden process.

**Example:**
```sh
warden clean
# Removing container: warden-build-deadbeef
# Removing container: warden-relay-deadbeef
# Removing network: warden-deadbeef
# Removed 3 orphaned resource(s).
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--runtime <name>` | Container runtime (finch, docker, podman) |
| `--color <mode>` | Color output (auto, always, never) |
| `-v, --verbose` | Show container runtime commands |
| `--version` | Print version |
| `-h, --help` | Help |
