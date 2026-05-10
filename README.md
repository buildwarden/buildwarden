# BuildWarden

[![CI](https://github.com/buildwarden/buildwarden/actions/workflows/main.yml/badge.svg)](https://github.com/buildwarden/buildwarden/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/buildwarden/buildwarden)](https://goreportcard.com/report/github.com/buildwarden/buildwarden)
[![Release](https://img.shields.io/github/v/release/buildwarden/buildwarden)](https://github.com/buildwarden/buildwarden/releases/latest)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

BuildWarden produces a cryptographically-signed, tamper-evident ledger of every network resource consumed during a software build. It enables build providers to independently verify build integrity — separately from the source author's own attestations.

## How It Works

BuildWarden orchestrates two containers on an isolated network:

```
┌─────────────────────────────────────────────────────────┐
│                    Host Machine                          │
│                                                         │
│  ┌─────────────┐         ┌─────────────────────────┐   │
│  │   Relay     │◄────────│    Build Container      │   │
│  │  10.0.87.2  │         │      10.0.87.3          │   │
│  │             │         │                         │   │
│  │  • DNS      │         │  • Docker-in-Docker     │   │
│  │  • HTTP/S   │  only   │  • Network isolated     │   │
│  │  • Ledger   │◄─conn──►│  • iptables enforced    │   │
│  │             │         │                         │   │
│  └──────┬──────┘         └─────────────────────────┘   │
│         │                                               │
│         ▼ external                                      │
│    ┌─────────┐                                          │
│    │ Internet│                                          │
│    └─────────┘                                          │
└─────────────────────────────────────────────────────────┘
```

- **Relay** — TLS-terminating MITM proxy that intercepts all HTTP/HTTPS traffic, records it to the ledger, and serves as DNS resolver. An ephemeral CA is generated per-build; its private key is destroyed with the container.
- **Build Container** — Rootless Docker-in-Docker environment. Network-isolated via iptables + route replacement so all traffic must flow through the relay.

The output is a binary ledger with chained Ed25519 signatures — altering, reordering, or removing any entry invalidates all subsequent signatures.

## Quick Start

### Install

Download a prebuilt binary from [Releases](https://github.com/buildwarden/buildwarden/releases), or build from source:

```sh
make build
```

### Run a build

```sh
# Use a Dockerfile in the current directory
warden build

# Specify a project directory
warden build ./my-project

# Specify a Dockerfile directly (context = parent directory)
warden build ./my-project/Dockerfile.prod
```

### Inspect a ledger

```sh
warden inspect /path/to/ledger

# Verbose output (shows individual records)
warden inspect --verbosity 1 /path/to/ledger

# Machine-readable output
warden inspect --json /path/to/ledger
```

## Configuration

BuildWarden looks for configuration in two places (merged in order):

1. `~/.config/warden/config.toml` — user defaults
2. `./warden.toml` — project overrides

```toml
[runtime]
cli = "finch"          # container runtime (finch, docker, podman)

[build]
dockerfile = ""        # override Dockerfile discovery
context = ""           # override context directory
capture = ""           # payload capture (none, headers, bodies, all)

[output]
color = "auto"         # auto, always, never
verbose = false
```

Environment variables override config: `WARDEN_CTR_CLI`, `NO_COLOR`, `WARDEN_VERBOSE`.

CLI flags override everything: `--runtime`, `--color`, `--capture`, `-v`.

### Runtime autodetection

If no runtime is configured, BuildWarden probes for: **finch** → docker → podman (first functional one wins).

## Ledger Format

The ledger is a binary file with:

- **Chained Ed25519 signatures** — each record signs over the previous signature
- **Multi-hash payload identity** — blake2b_256, sha256, sha1, md5 for every payload
- **Redactable metadata** — CBOR-encoded, not part of signatures; can be stripped without affecting integrity
- **Artifact records** — build outputs are structurally distinguished from network inputs

See [docs/design/Ledger-Spec.md](docs/design/Ledger-Spec.md) for the full specification.

## Demo Cases

| Dockerfile | Description | Time |
|-----------|-------------|------|
| `examples/Dockerfile.simple` | Build Python `requests` wheel from source, post artifact | ~1 min |
| `examples/Dockerfile.expanded` | Build + test + type-check requests (shows ledger truncatability) | ~3 min |
| `examples/Dockerfile.cryptography` | Python `cryptography` from source (Rust/Cargo + pip + apt) | ~10 min |
| `examples/Dockerfile.pytorch-aarch64` | PyTorch from source with CUDA (pinnacle scale test) | ~3 hrs |

```sh
warden build examples/Dockerfile.simple
```

## Design Documents

- [Ledger Specification](docs/design/Ledger-Spec.md)
- [Philosophy](docs/design/Philosophy.md)
- [Initial Proposal](docs/design/Initial-Proposal.md)

## Development

See [DEVELOPING.md](DEVELOPING.md) for build instructions, project structure, and contribution guidelines.

## License

Apache-2.0
